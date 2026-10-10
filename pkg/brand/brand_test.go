package brand_test

import (
	"slices"
	"testing"
	"unicode/utf8"

	"github.com/achandrapaul/digest/pkg/brand"
)

func TestBannerRowsRunTopToBottomAtOneWidth(t *testing.T) {
	want := []string{brand.BannerTopRow, brand.BannerMiddleRow, brand.BannerBottomRow}
	if !slices.Equal(brand.BannerRows, want) {
		t.Fatalf("BannerRows = %q, want %q", brand.BannerRows, want)
	}
	for _, row := range brand.BannerRows {
		if width := utf8.RuneCountInString(row); width != utf8.RuneCountInString(brand.BannerTopRow) {
			t.Errorf("row %q is %d wide", row, width)
		}
	}
}
