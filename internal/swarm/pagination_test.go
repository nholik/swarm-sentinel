package swarm

import (
	"fmt"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/filters"
)

type stubItem struct {
	ID string
}

func TestPaginateByIDPrefix_Small(t *testing.T) {
	data := []stubItem{
		{ID: "a1"},
		{ID: "b2"},
		{ID: "c3"},
	}

	listFn := func(f filters.Args) ([]stubItem, error) {
		prefix := firstFilterValue(f, "id")
		return filterByPrefix(data, prefix), nil
	}

	items, err := paginateByIDPrefix(filters.NewArgs(), listFn, func(item stubItem) string {
		return item.ID
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(items) != len(data) {
		t.Fatalf("expected %d items, got %d", len(data), len(items))
	}

	seen := make(map[string]struct{})
	for _, item := range items {
		seen[item.ID] = struct{}{}
	}
	for _, item := range data {
		if _, ok := seen[item.ID]; !ok {
			t.Fatalf("missing item %s", item.ID)
		}
	}
}

func TestPaginateByIDPrefix_LargePrefix(t *testing.T) {
	data := make([]stubItem, maxListPageSize+1)
	for i := range data {
		data[i] = stubItem{ID: fmt.Sprintf("a0%04d", i)}
	}

	listFn := func(f filters.Args) ([]stubItem, error) {
		prefix := firstFilterValue(f, "id")
		return filterByPrefix(data, prefix), nil
	}

	items, err := paginateByIDPrefix(filters.NewArgs(), listFn, func(item stubItem) string {
		return item.ID
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(items) != len(data) {
		t.Fatalf("expected %d items, got %d", len(data), len(items))
	}
}

func filterByPrefix(data []stubItem, prefix string) []stubItem {
	if prefix == "" {
		return append([]stubItem(nil), data...)
	}
	filtered := make([]stubItem, 0, len(data))
	for _, item := range data {
		if strings.HasPrefix(item.ID, prefix) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func firstFilterValue(f filters.Args, key string) string {
	values := f.Get(key)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
