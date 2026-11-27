package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/kipit-labs/kipitrek/internal/domain"
)

func (r *HikeRepository) GetTrack(ctx context.Context, id uuid.UUID, tolerance float64) ([]domain.Segment, error) {
	var row struct{ Geom []byte }
	err := r.db.WithContext(ctx).
		Model(&HikeModel{}).
		Select("ST_AsBinary(ST_Force2D(ST_SimplifyPreserveTopology(geom, ?))) AS geom", tolerance).
		Where("id = ?", id).
		Take(&row).Error
	if err != nil {
		return nil, mapErr(err)
	}
	return wkbToSegments(row.Geom)
}

func (r *HikeRepository) ListTracks(ctx context.Context, userID uuid.UUID, tolerance float64) ([]domain.HikeTrack, error) {
	var rows []struct {
		ID   uuid.UUID
		Name string
		Geom []byte
	}
	err := r.db.WithContext(ctx).
		Model(&HikeModel{}).
		Select("id, name, ST_AsBinary(ST_Force2D(ST_SimplifyPreserveTopology(geom, ?))) AS geom", tolerance).
		Where(ownedOrTagged, userID, userID).
		Order(hikeOrder).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.HikeTrack, 0, len(rows))
	for _, row := range rows {
		segs, err := wkbToSegments(row.Geom)
		if err != nil {
			return nil, fmt.Errorf("decode hike %s geometry: %w", row.ID, err)
		}
		out = append(out, domain.HikeTrack{ID: row.ID, Name: row.Name, Segments: segs})
	}
	return out, nil
}

func (r *HikeRepository) ListOutdated(ctx context.Context, version, limit int) ([]domain.Hike, error) {
	var ms []HikeModel
	err := r.db.WithContext(ctx).
		Select("id, gpx_raw").
		Where("derived_version < ?", version).
		Order("id").
		Limit(limit).
		Find(&ms).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.Hike, len(ms))
	for i, m := range ms {
		out[i] = domain.Hike{ID: m.ID, RawGPX: m.GPXRaw}
	}
	return out, nil
}

func (r *HikeRepository) SaveDerived(ctx context.Context, id uuid.UUID, d domain.HikeDerived, version int) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Model(&HikeModel{}).
			Where("id = ?", id).
			Updates(map[string]any{
				"elevation_loss_m": d.ElevationLossM,
				"min_ele_m":        d.MinEleM,
				"max_ele_m":        d.MaxEleM,
				"moving_s":         d.MovingS,
				"derived_version":  version,
			}).Error
		if err != nil {
			return err
		}
		if err := tx.Where("hike_id = ?", id).Delete(&HikeBestEffortModel{}).Error; err != nil {
			return err
		}
		if err := createBestEfforts(tx, id, d.BestEfforts); err != nil {
			return err
		}
		if err := tx.Where("hike_id = ?", id).Delete(&HikeTileModel{}).Error; err != nil {
			return err
		}
		return createTiles(tx, id, d.Tiles)
	})
}

func (r *HikeRepository) ReplaceTrack(ctx context.Context, userID, id uuid.UUID, h *domain.Hike) error {
	g, err := segmentsToGeom(h.Segments)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&HikeModel{}).
			Where("user_id = ? AND id = ?", userID, id).
			Updates(map[string]any{
				"planned":          false,
				"distance_m":       h.DistanceM,
				"elevation_gain_m": h.ElevationGainM,
				"started_at":       h.StartedAt,
				"duration_s":       h.DurationS,
				"elevation_loss_m": h.ElevationLossM,
				"min_ele_m":        h.MinEleM,
				"max_ele_m":        h.MaxEleM,
				"moving_s":         h.MovingS,
				"derived_version":  h.DerivedVersion,
				"weather":          nil,
				"weather_at":       nil,
				"min_lon":          h.Bounds.MinLon,
				"min_lat":          h.Bounds.MinLat,
				"max_lon":          h.Bounds.MaxLon,
				"max_lat":          h.Bounds.MaxLat,
				"geom":             g,
				"gpx_raw":          h.RawGPX,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return domain.ErrNotFound
		}
		if err := tx.Where("hike_id = ?", id).Delete(&HikeBestEffortModel{}).Error; err != nil {
			return err
		}
		if err := createBestEfforts(tx, id, h.BestEfforts); err != nil {
			return err
		}
		if err := tx.Where("hike_id = ?", id).Delete(&HikeTileModel{}).Error; err != nil {
			return err
		}
		return createTiles(tx, id, h.Tiles)
	})
}

func (r *HikeRepository) ListTiles(ctx context.Context, userID uuid.UUID) (map[uuid.UUID][]domain.Tile, error) {
	var ms []HikeTileModel
	err := r.db.WithContext(ctx).
		Where("hike_id IN (SELECT id FROM hikes WHERE NOT planned AND ("+ownedOrTagged+"))", userID, userID).
		Order("hike_id, x, y").
		Find(&ms).Error
	if err != nil {
		return nil, err
	}
	out := map[uuid.UUID][]domain.Tile{}
	for _, m := range ms {
		out[m.HikeID] = append(out[m.HikeID], domain.Tile{X: m.X, Y: m.Y})
	}
	return out, nil
}

func (r *HikeRepository) ListSimilar(ctx context.Context, userID, hikeID uuid.UUID, maxDeviationM float64) ([]domain.Hike, error) {
	var ms []HikeModel
	err := r.db.WithContext(ctx).
		Scopes(withOwner, withDetails).
		Omit("geom", "gpx_raw").
		Where("id IN ("+similarHikes+")", map[string]any{"hike": hikeID, "user": userID, "max": maxDeviationM}).
		Order(hikeOrder).
		Find(&ms).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.Hike, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return out, nil
}
