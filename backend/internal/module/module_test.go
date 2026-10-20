package module

import (
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"nicetools/backend/internal/model"
)

type stub struct{ id string }

func (s stub) ID() string                                         { return s.id }
func (s stub) Migrate(db *gorm.DB) error                          { return nil }
func (s stub) Register(read, write, pub *gin.RouterGroup, d Deps) {}

func TestVerifyCatalog(t *testing.T) {
	entries := map[string]model.Catalog{
		"pure-front":  {ID: "pure-front"},
		"with-server": {ID: "with-server", Backend: true},
	}
	if err := VerifyCatalog([]Module{stub{"with-server"}}, entries); err != nil {
		t.Fatalf("aligned setup rejected: %v", err)
	}
	if err := VerifyCatalog([]Module{stub{"pure-front"}}, entries); err == nil {
		t.Fatal("module without backend flag accepted")
	}
	if err := VerifyCatalog([]Module{stub{"ghost"}}, entries); err == nil {
		t.Fatal("module missing from catalog accepted")
	}
	if err := VerifyCatalog(nil, entries); err == nil {
		t.Fatal("backend tool without module accepted")
	}
}
