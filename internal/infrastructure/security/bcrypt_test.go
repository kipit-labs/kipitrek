package security

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestBcryptHasher(t *testing.T) {
	h := NewBcryptHasher(bcrypt.MinCost)
	hash, err := h.Hash("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(hash, "correct horse") {
		t.Fatal("hash contains the password")
	}
	if err := h.Compare(hash, "correct horse"); err != nil {
		t.Errorf("right password rejected: %v", err)
	}
	if err := h.Compare(hash, "wrong horse"); err == nil {
		t.Error("wrong password accepted")
	}
	if other, _ := h.Hash("correct horse"); other == hash {
		t.Error("hashes are not salted")
	}
	if NewBcryptHasher(0).cost != bcrypt.DefaultCost {
		t.Error("zero cost does not default")
	}
}
