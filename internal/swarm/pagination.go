package swarm

import "github.com/docker/docker/api/types/filters"

const (
	maxListPageSize    = 1000
	maxIDPrefixDepth   = 2
	idPrefixCharacters = "0123456789abcdefghijklmnopqrstuvwxyz"
)

func paginateByIDPrefix[T any](base filters.Args, listFn func(filters.Args) ([]T, error), idFn func(T) string) ([]T, error) {
	results := make([]T, 0)
	seen := make(map[string]struct{})

	for _, ch := range idPrefixCharacters {
		prefix := string(ch)
		items, err := paginateByIDPrefixDepth(base, prefix, 1, listFn, idFn)
		if err != nil {
			return nil, err
		}
		results = appendUnique(results, items, seen, idFn)
	}

	return results, nil
}

func paginateByIDPrefixDepth[T any](base filters.Args, prefix string, depth int, listFn func(filters.Args) ([]T, error), idFn func(T) string) ([]T, error) {
	query := base.Clone()
	query.Add("id", prefix)

	items, err := listFn(query)
	if err != nil {
		return nil, err
	}
	if len(items) <= maxListPageSize || depth >= maxIDPrefixDepth {
		return items, nil
	}

	results := make([]T, 0, len(items))
	seen := make(map[string]struct{})

	for _, ch := range idPrefixCharacters {
		childPrefix := prefix + string(ch)
		childItems, err := paginateByIDPrefixDepth(base, childPrefix, depth+1, listFn, idFn)
		if err != nil {
			return nil, err
		}
		results = appendUnique(results, childItems, seen, idFn)
	}

	return results, nil
}

func appendUnique[T any](dst []T, items []T, seen map[string]struct{}, idFn func(T) string) []T {
	for _, item := range items {
		id := idFn(item)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		dst = append(dst, item)
	}
	return dst
}
