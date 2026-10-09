package contract

import (
	"os"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"

	"github.com/amuxos/agent-pack-contract/go/internal/errs"
	"github.com/amuxos/agent-pack-contract/go/internal/jsonx"
)

// loadToml parses a TOML file into map[string]any (key order irrelevant).
// Use loadTomlOrdered for files whose key order reaches the manifest JSON.
func loadToml(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errs.New("missing %s", path)
	}
	var out map[string]any
	if err := toml.Unmarshal(data, &out); err != nil {
		return nil, errs.New("%s: invalid TOML: %v", path, err)
	}
	return out, nil
}

// loadTomlOrdered parses TOML preserving source key order. Provider option key
// order flows into the manifest JSON (byte-critical), so a
// plain toml.Unmarshal into map[string]any is NOT enough for provider files.
func loadTomlOrdered(path string) (*jsonx.OrderedMap, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errs.New("missing %s", path)
	}
	p := &unstable.Parser{}
	p.Reset(data)

	root := jsonx.NewMap()
	current := root
	for p.NextExpression() {
		e := p.Expression()
		switch e.Kind {
		case unstable.Table:
			var err error
			current, err = navigateTable(root, keyPath(e))
			if err != nil {
				return nil, errs.New("%s: invalid TOML: %v", path, err)
			}
		case unstable.ArrayTable:
			return nil, errs.New("%s: array tables are not supported", path)
		case unstable.KeyValue:
			if err := insertKeyValue(current, e); err != nil {
				return nil, errs.New("%s: invalid TOML: %v", path, err)
			}
		}
	}
	if err := p.Error(); err != nil {
		return nil, errs.New("%s: invalid TOML: %v", path, err)
	}
	return root, nil
}

func keyPath(n *unstable.Node) []string {
	var parts []string
	for it := n.Key(); it.Next(); {
		parts = append(parts, string(it.Node().Data))
	}
	return parts
}

func navigateTable(root *jsonx.OrderedMap, path []string) (*jsonx.OrderedMap, error) {
	cur := root
	for _, p := range path {
		if next, ok := cur.Get(p); ok {
			m, ok := next.(*jsonx.OrderedMap)
			if !ok {
				return nil, errs.New("key %q is already used as a value and cannot be a table", p)
			}
			cur = m
		} else {
			nm := jsonx.NewMap()
			cur.Set(p, nm)
			cur = nm
		}
	}
	return cur, nil
}

func insertKeyValue(cur *jsonx.OrderedMap, kv *unstable.Node) error {
	path := keyPath(kv)
	val, err := decodeTomlValue(kv.Value())
	if err != nil {
		return err
	}
	for _, p := range path[:len(path)-1] {
		if next, ok := cur.Get(p); ok {
			m, ok := next.(*jsonx.OrderedMap)
			if !ok {
				return errs.New("key %q is already used as a value and cannot be a table", p)
			}
			cur = m
		} else {
			nm := jsonx.NewMap()
			cur.Set(p, nm)
			cur = nm
		}
	}
	cur.Set(path[len(path)-1], val)
	return nil
}

func decodeTomlValue(n *unstable.Node) (any, error) {
	switch n.Kind {
	case unstable.String:
		return string(n.Data), nil
	case unstable.Bool:
		return string(n.Data) == "true", nil
	case unstable.Integer:
		s := strings.ReplaceAll(string(n.Data), "_", "")
		return strconv.ParseInt(s, 0, 64)
	case unstable.Float:
		s := strings.ReplaceAll(string(n.Data), "_", "")
		return strconv.ParseFloat(s, 64)
	case unstable.DateTime, unstable.LocalDate, unstable.LocalTime, unstable.LocalDateTime:
		// Contract files never use datetimes; keep the raw literal so callers
		// see a non-nil placeholder rather than failing.
		return string(n.Data), nil
	case unstable.Array:
		arr := []any{}
		for it := n.Children(); it.Next(); {
			v, err := decodeTomlValue(it.Node())
			if err != nil {
				return nil, err
			}
			arr = append(arr, v)
		}
		return arr, nil
	case unstable.InlineTable:
		m := jsonx.NewMap()
		for it := n.Children(); it.Next(); {
			kv := it.Node()
			k := singleKey(kv)
			v, err := decodeTomlValue(kv.Value())
			if err != nil {
				return nil, err
			}
			m.Set(k, v)
		}
		return m, nil
	default:
		return nil, errs.New("unexpected TOML node kind %s", n.Kind)
	}
}

func singleKey(kv *unstable.Node) string {
	it := kv.Key()
	if it.Next() {
		return string(it.Node().Data)
	}
	return ""
}
