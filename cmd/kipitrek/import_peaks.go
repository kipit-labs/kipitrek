package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/kipit-labs/kipitrek/internal/config"
	"github.com/kipit-labs/kipitrek/internal/domain"
	"github.com/kipit-labs/kipitrek/internal/infrastructure/overpass"
	"github.com/kipit-labs/kipitrek/internal/infrastructure/postgres"
	"github.com/kipit-labs/kipitrek/internal/usecase/summit"
)

// importPeaks loads named OpenStreetMap peaks in a bounding box into the
// database: `kipitrek import-peaks -bbox minLon,minLat,maxLon,maxLat`.
func importPeaks(args []string) error {
	fs := flag.NewFlagSet("import-peaks", flag.ContinueOnError)
	bboxFlag := fs.String("bbox", "", "minLon,minLat,maxLon,maxLat (WGS84)")
	endpoint := fs.String("overpass", overpass.DefaultURL, "Overpass API URL")
	if err := fs.Parse(args); err != nil {
		return err
	}
	bbox, err := parseBBox(*bboxFlag)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	if err := postgres.Migrate(db); err != nil {
		return err
	}
	n, err := summit.NewService(postgres.NewPeakRepository(db), overpass.NewClient(*endpoint), nil).Import(ctx, bbox)
	if err != nil {
		return err
	}
	slog.Info("imported peaks", "count", n, "bbox", *bboxFlag)
	return nil
}

func parseBBox(s string) (domain.Bounds, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 4 {
		return domain.Bounds{}, fmt.Errorf("-bbox needs minLon,minLat,maxLon,maxLat, got %q", s)
	}
	var v [4]float64
	for i, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return domain.Bounds{}, fmt.Errorf("-bbox: %w", err)
		}
		v[i] = f
	}
	return domain.Bounds{MinLon: v[0], MinLat: v[1], MaxLon: v[2], MaxLat: v[3]}, nil
}
