package config

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
)

// managedValue is one value owned by grok-switch. Paths are absolute TOML
// paths, regardless of whether the source uses table headers or dotted keys.
type managedValue struct {
	path  []string
	value interface{}
}

type tomlExpression struct {
	path       []string
	tablePath  []string
	keyStart   int
	valueStart int
	valueEnd   int
}

type tomlTable struct {
	path        []string
	headerStart int
	headerEnd   int
	insertAt    int
}

type tomlLayout struct {
	expressions []tomlExpression
	byPath      map[string]int
	tables      map[string]tomlTable
	rootInsert  int
}

type sourcePatch struct {
	start int
	end   int
	repl  []byte
}

var managedModelKeys = map[string]bool{
	"model":                     true,
	"base_url":                  true,
	"api_backend":               true,
	"api_key":                   true,
	"supports_reasoning_effort": true,
	"reasoning_effort":          true,
	"reasoning_efforts":         true,
}

// rewriteManagedTOML changes only grok-switch-owned keys. All unrelated source
// bytes (including comments, whitespace, ordering, and unknown fields inside
// managed tables) are retained verbatim.
func rewriteManagedTOML(raw []byte, desired []managedValue) ([]byte, error) {
	layout, err := parseTOMLLayout(raw)
	if err != nil {
		return nil, err
	}

	desiredByPath := make(map[string]managedValue, len(desired))
	desiredTables := make(map[string]bool)
	for _, item := range desired {
		desiredByPath[pathKey(item.path)] = item
		desiredTables[pathKey(item.path[:len(item.path)-1])] = true
	}

	removed := make(map[int]bool)
	var patches []sourcePatch
	found := make(map[string]bool)

	for i, expr := range layout.expressions {
		key := pathKey(expr.path)
		if item, ok := desiredByPath[key]; ok {
			encoded, err := encodeTOMLValue(item.value)
			if err != nil {
				return nil, err
			}
			patches = append(patches, sourcePatch{
				start: expr.valueStart,
				end:   expr.valueEnd,
				repl:  encoded,
			})
			found[key] = true
			continue
		}

		// Managed keys in model sections that are not part of the new profile
		// are stale. Remove just those assignments and leave unknown fields.
		if isManagedModelPath(expr.path) {
			removed[i] = true
			patches = append(patches, removeExpressionPatch(raw, expr))
		}
	}

	missingByTable := make(map[string][]managedValue)
	var tableOrder []string
	for _, item := range desired {
		key := pathKey(item.path)
		if found[key] {
			continue
		}
		parent := pathKey(item.path[:len(item.path)-1])
		if _, exists := missingByTable[parent]; !exists {
			tableOrder = append(tableOrder, parent)
		}
		missingByTable[parent] = append(missingByTable[parent], item)
	}

	insertions := make(map[int][]byte)
	var newTables bytes.Buffer
	for _, tableKey := range tableOrder {
		items := missingByTable[tableKey]
		if table, ok := layout.tables[tableKey]; ok {
			for _, item := range items {
				line, err := assignmentLine(item.path[len(item.path)-1:], item.value)
				if err != nil {
					return nil, err
				}
				insertions[table.insertAt] = append(insertions[table.insertAt], line...)
			}
			continue
		}

		parentPath := items[0].path[:len(items[0].path)-1]
		if parentUsesDottedKeys(layout, parentPath) {
			for _, item := range items {
				line, err := assignmentLine(item.path, item.value)
				if err != nil {
					return nil, err
				}
				insertions[layout.rootInsert] = append(insertions[layout.rootInsert], line...)
			}
			continue
		}
		if parentIsInlineTable(layout, parentPath) {
			return nil, fmt.Errorf("暂不支持修改内联 TOML 表 %s，请改用 [%s] 表格式",
				formatKeyPath(parentPath), formatKeyPath(parentPath))
		}

		newTables.WriteString("[")
		newTables.WriteString(formatKeyPath(parentPath))
		newTables.WriteString("]\n")
		for _, item := range items {
			line, err := assignmentLine(item.path[len(item.path)-1:], item.value)
			if err != nil {
				return nil, err
			}
			newTables.Write(line)
		}
		newTables.WriteByte('\n')
	}

	if newTables.Len() > 0 {
		insertions[len(raw)] = append(insertions[len(raw)], newTables.Bytes()...)
	}
	for pos, content := range insertions {
		content = adaptGeneratedNewlines(raw, content)
		patches = append(patches, sourcePatch{
			start: pos,
			end:   pos,
			repl:  insertionPatch(raw, pos, content),
		})
	}

	// Remove table headers that would otherwise become empty after stale
	// managed model keys are removed. Comments in those tables remain intact.
	for key, table := range layout.tables {
		if desiredTables[key] || !isModelTable(table.path) {
			continue
		}
		if tableHasRemainingValues(layout, table.path, removed) {
			continue
		}
		patches = append(patches, blankLinePatch(raw, table.headerStart, table.headerEnd))
	}

	out := applySourcePatches(raw, patches)
	if err := validateTOMLBytes(out); err != nil {
		return nil, fmt.Errorf("更新后的 TOML 无法解析: %w", err)
	}
	return out, nil
}

// removeManagedTOML removes all grok-switch-owned keys for official mode,
// while retaining unknown fields and source formatting.
func removeManagedTOML(raw []byte) ([]byte, error) {
	layout, err := parseTOMLLayout(raw)
	if err != nil {
		return nil, err
	}

	removed := make(map[int]bool)
	var patches []sourcePatch
	for i, expr := range layout.expressions {
		if isTopLevelManagedPath(expr.path) || isManagedModelPath(expr.path) {
			removed[i] = true
			patches = append(patches, removeExpressionPatch(raw, expr))
		}
	}

	// Work deepest-first so empty parent tables such as [subagents] and
	// [model] can also be removed after their managed child tables disappear.
	var tables []tomlTable
	for _, table := range layout.tables {
		tables = append(tables, table)
	}
	sort.Slice(tables, func(i, j int) bool { return len(tables[i].path) > len(tables[j].path) })
	removedTables := make(map[string]bool)
	for _, table := range tables {
		if !isManagedTableFamily(table.path) || tableHasRemainingValues(layout, table.path, removed) {
			continue
		}
		hasChild := false
		for _, child := range tables {
			if removedTables[pathKey(child.path)] || len(child.path) <= len(table.path) {
				continue
			}
			if hasPathPrefix(child.path, table.path) {
				hasChild = true
				break
			}
		}
		if hasChild {
			continue
		}
		removedTables[pathKey(table.path)] = true
		patches = append(patches, blankLinePatch(raw, table.headerStart, table.headerEnd))
	}

	out := applySourcePatches(raw, patches)
	if err := validateTOMLBytes(out); err != nil {
		return nil, fmt.Errorf("更新后的 TOML 无法解析: %w", err)
	}
	return out, nil
}

func parseTOMLLayout(raw []byte) (tomlLayout, error) {
	layout := tomlLayout{
		byPath:     make(map[string]int),
		tables:     make(map[string]tomlTable),
		rootInsert: len(raw),
	}
	var parser unstable.Parser
	parser.Reset(raw)
	var current []string
	var previousTableKey string
	firstTable := true

	for parser.NextExpression() {
		node := parser.Expression()
		switch node.Kind {
		case unstable.Table, unstable.ArrayTable:
			parts, keyStart, _ := nodeKey(node)
			headerStart := lineStart(raw, keyStart)
			if firstTable {
				layout.rootInsert = headerStart
				firstTable = false
			}
			if previousTableKey != "" {
				table := layout.tables[previousTableKey]
				table.insertAt = headerStart
				layout.tables[previousTableKey] = table
			}
			current = append([]string(nil), parts...)
			if node.Kind == unstable.Table {
				key := pathKey(parts)
				layout.tables[key] = tomlTable{
					path:        append([]string(nil), parts...),
					headerStart: headerStart,
					headerEnd:   lineEnd(raw, keyStart),
					insertAt:    len(raw),
				}
				previousTableKey = key
			} else {
				previousTableKey = ""
			}

		case unstable.KeyValue:
			parts, keyStart, keyEnd := nodeKey(node)
			path := append(append([]string(nil), current...), parts...)
			valueStart, valueEnd, err := scanTOMLValueRange(raw, keyEnd)
			if err != nil {
				return layout, err
			}
			expr := tomlExpression{
				path:       path,
				tablePath:  append([]string(nil), current...),
				keyStart:   keyStart,
				valueStart: valueStart,
				valueEnd:   valueEnd,
			}
			layout.byPath[pathKey(path)] = len(layout.expressions)
			layout.expressions = append(layout.expressions, expr)
		}
	}
	if err := parser.Error(); err != nil {
		return layout, err
	}
	return layout, nil
}

func nodeKey(node *unstable.Node) ([]string, int, int) {
	var parts []string
	start := -1
	end := -1
	it := node.Key()
	for it.Next() {
		key := it.Node()
		if start < 0 {
			start = int(key.Raw.Offset)
		}
		end = int(key.Raw.Offset + key.Raw.Length)
		parts = append(parts, string(key.Data))
	}
	return parts, start, end
}

func scanTOMLValueRange(raw []byte, keyEnd int) (int, int, error) {
	i := keyEnd
	for i < len(raw) && (raw[i] == ' ' || raw[i] == '\t') {
		i++
	}
	if i >= len(raw) || raw[i] != '=' {
		return 0, 0, fmt.Errorf("无法定位 TOML 赋值符")
	}
	i++
	for i < len(raw) && (raw[i] == ' ' || raw[i] == '\t') {
		i++
	}
	if i >= len(raw) {
		return 0, 0, fmt.Errorf("无法定位 TOML 值")
	}
	start := i
	switch raw[i] {
	case '\'', '"':
		end, err := scanTOMLString(raw, i)
		return start, end, err
	case '[', '{':
		end, err := scanTOMLContainer(raw, i)
		return start, end, err
	default:
		for i < len(raw) && raw[i] != '\n' && raw[i] != '\r' && raw[i] != '#' {
			i++
		}
		for i > start && (raw[i-1] == ' ' || raw[i-1] == '\t') {
			i--
		}
		return start, i, nil
	}
}

func scanTOMLString(raw []byte, start int) (int, error) {
	quote := raw[start]
	triple := start+2 < len(raw) && raw[start+1] == quote && raw[start+2] == quote
	i := start + 1
	if triple {
		i = start + 3
	}
	for i < len(raw) {
		if triple {
			if i+2 < len(raw) && raw[i] == quote && raw[i+1] == quote && raw[i+2] == quote {
				return i + 3, nil
			}
		} else if raw[i] == quote {
			return i + 1, nil
		}
		if quote == '"' && raw[i] == '\\' {
			i += 2
			continue
		}
		i++
	}
	return 0, fmt.Errorf("未终止的 TOML 字符串")
}

func scanTOMLContainer(raw []byte, start int) (int, error) {
	stack := []byte{raw[start]}
	for i := start + 1; i < len(raw); {
		switch raw[i] {
		case '\'', '"':
			end, err := scanTOMLString(raw, i)
			if err != nil {
				return 0, err
			}
			i = end
		case '#':
			if newline := bytes.IndexByte(raw[i:], '\n'); newline >= 0 {
				i += newline + 1
			} else {
				return 0, fmt.Errorf("未终止的 TOML 容器")
			}
		case '[', '{':
			stack = append(stack, raw[i])
			i++
		case ']', '}':
			open := stack[len(stack)-1]
			if (open == '[' && raw[i] != ']') || (open == '{' && raw[i] != '}') {
				return 0, fmt.Errorf("TOML 容器括号不匹配")
			}
			stack = stack[:len(stack)-1]
			i++
			if len(stack) == 0 {
				return i, nil
			}
		default:
			i++
		}
	}
	return 0, fmt.Errorf("未终止的 TOML 容器")
}

func isTopLevelManagedPath(path []string) bool {
	return equalPath(path, []string{"endpoints", "api_url"}) ||
		equalPath(path, []string{"models", "default"}) ||
		equalPath(path, []string{"models", "web_search"}) ||
		equalPath(path, []string{"subagents", "models", "explore"}) ||
		equalPath(path, []string{"subagents", "models", "plan"})
}

func isManagedModelPath(path []string) bool {
	return len(path) == 3 && path[0] == "model" && managedModelKeys[path[2]]
}

func isModelTable(path []string) bool {
	return len(path) == 2 && path[0] == "model"
}

func isManagedTableFamily(path []string) bool {
	if len(path) == 0 {
		return false
	}
	switch path[0] {
	case "endpoints", "models", "subagents", "model":
		return true
	default:
		return false
	}
}

func tableHasRemainingValues(layout tomlLayout, tablePath []string, removed map[int]bool) bool {
	for i, expr := range layout.expressions {
		if removed[i] {
			continue
		}
		if equalPath(expr.tablePath, tablePath) {
			return true
		}
	}
	return false
}

func parentUsesDottedKeys(layout tomlLayout, parent []string) bool {
	for _, expr := range layout.expressions {
		if len(expr.tablePath) == 0 && len(expr.path) > len(parent) && hasPathPrefix(expr.path, parent) {
			return true
		}
	}
	return false
}

func parentIsInlineTable(layout tomlLayout, parent []string) bool {
	_, ok := layout.byPath[pathKey(parent)]
	return ok
}

func assignmentLine(path []string, value interface{}) ([]byte, error) {
	encoded, err := encodeTOMLValue(value)
	if err != nil {
		return nil, err
	}
	return []byte(formatKeyPath(path) + " = " + string(encoded) + "\n"), nil
}

func encodeTOMLValue(value interface{}) ([]byte, error) {
	raw, err := toml.Marshal(map[string]interface{}{"value": value})
	if err != nil {
		return nil, fmt.Errorf("序列化 TOML 值失败: %w", err)
	}
	line := strings.TrimSpace(string(raw))
	idx := strings.Index(line, "=")
	if idx < 0 {
		return nil, fmt.Errorf("序列化 TOML 值失败: 缺少赋值符")
	}
	return []byte(strings.TrimSpace(line[idx+1:])), nil
}

func formatKeyPath(path []string) string {
	parts := make([]string, len(path))
	for i, part := range path {
		if isBareKey(part) {
			parts[i] = part
			continue
		}
		encoded, err := encodeTOMLValue(part)
		if err != nil {
			parts[i] = fmt.Sprintf("%q", part)
		} else {
			parts[i] = string(encoded)
		}
	}
	return strings.Join(parts, ".")
}

func isBareKey(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func removeExpressionPatch(raw []byte, expr tomlExpression) sourcePatch {
	start := lineStart(raw, expr.keyStart)
	end := lineEnd(raw, expr.valueEnd)
	return sourcePatch{start: start, end: end, repl: preserveCommentsAndLines(raw[start:end])}
}

func blankLinePatch(raw []byte, start, end int) sourcePatch {
	return sourcePatch{start: start, end: end, repl: preserveCommentsAndLines(raw[start:end])}
}

func preserveCommentsAndLines(segment []byte) []byte {
	var out []byte
	quote := byte(0)
	triple := false
	escaped := false
	for pos := 0; pos < len(segment); {
		lineEndPos := bytes.IndexByte(segment[pos:], '\n')
		end := len(segment)
		if lineEndPos >= 0 {
			end = pos + lineEndPos + 1
		}
		line := segment[pos:end]
		contentEnd := len(line)
		for contentEnd > 0 && (line[contentEnd-1] == '\n' || line[contentEnd-1] == '\r') {
			contentEnd--
		}
		comment := -1
		for i := 0; i < contentEnd; i++ {
			ch := line[i]
			if quote != 0 {
				if quote == '"' && escaped {
					escaped = false
					continue
				}
				if quote == '"' && ch == '\\' {
					escaped = true
					continue
				}
				if triple {
					if i+2 < contentEnd && ch == quote && line[i+1] == quote && line[i+2] == quote {
						quote = 0
						triple = false
						i += 2
					}
				} else if ch == quote {
					quote = 0
				}
				continue
			}
			if ch == '\'' || ch == '"' {
				quote = ch
				triple = i+2 < contentEnd && line[i+1] == ch && line[i+2] == ch
				if triple {
					i += 2
				}
				continue
			}
			if ch == '#' {
				comment = i
				break
			}
		}
		if comment >= 0 {
			indentEnd := 0
			for indentEnd < contentEnd && (line[indentEnd] == ' ' || line[indentEnd] == '\t') {
				indentEnd++
			}
			out = append(out, line[:indentEnd]...)
			out = append(out, line[comment:contentEnd]...)
		}
		out = append(out, line[contentEnd:]...)
		pos = end
	}
	return out
}

func adaptGeneratedNewlines(raw, generated []byte) []byte {
	if !bytes.Contains(raw, []byte("\r\n")) {
		return generated
	}
	return bytes.ReplaceAll(generated, []byte("\n"), []byte("\r\n"))
}

func insertionPatch(raw []byte, pos int, content []byte) []byte {
	if len(content) == 0 {
		return nil
	}
	var out []byte
	if pos > 0 && raw[pos-1] != '\n' {
		out = append(out, '\n')
	}
	// New table blocks get a visual separator without disturbing existing
	// blank lines elsewhere in the document.
	if pos == len(raw) && pos > 0 && !bytes.HasSuffix(raw[:pos], []byte("\n\n")) && content[0] == '[' {
		out = append(out, '\n')
	}
	out = append(out, content...)
	return out
}

func applySourcePatches(raw []byte, patches []sourcePatch) []byte {
	sort.SliceStable(patches, func(i, j int) bool {
		if patches[i].start == patches[j].start {
			// Replacements at an offset must run before insertions at that same
			// offset, otherwise the replacement could consume inserted bytes.
			return patches[i].end > patches[j].end
		}
		return patches[i].start > patches[j].start
	})
	out := append([]byte(nil), raw...)
	for _, patch := range patches {
		next := make([]byte, 0, len(out)-(patch.end-patch.start)+len(patch.repl))
		next = append(next, out[:patch.start]...)
		next = append(next, patch.repl...)
		next = append(next, out[patch.end:]...)
		out = next
	}
	return out
}

func validateTOMLBytes(raw []byte) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var data map[string]interface{}
	return toml.Unmarshal(raw, &data)
}

func lineStart(raw []byte, offset int) int {
	if offset < 0 {
		return 0
	}
	idx := bytes.LastIndexByte(raw[:offset], '\n')
	return idx + 1
}

// lineEnd returns the first byte after the line terminator containing offset.
func lineEnd(raw []byte, offset int) int {
	if offset < 0 {
		return 0
	}
	if offset > len(raw) {
		offset = len(raw)
	}
	idx := bytes.IndexByte(raw[offset:], '\n')
	if idx < 0 {
		return len(raw)
	}
	return offset + idx + 1
}

func pathKey(path []string) string { return strings.Join(path, "\x00") }

func equalPath(a, b []string) bool {
	return len(a) == len(b) && hasPathPrefix(a, b)
}

func hasPathPrefix(path, prefix []string) bool {
	if len(path) < len(prefix) {
		return false
	}
	for i := range prefix {
		if path[i] != prefix[i] {
			return false
		}
	}
	return true
}
