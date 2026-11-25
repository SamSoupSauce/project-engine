package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type jsonUINode struct {
	ID       string         `json:"id"`
	Style    jsonStyle      `json:"style"`
	OnClick  string         `json:"on_click"`
	OnHover  string         `json:"on_hover"`
	Text     string         `json:"text"`
	Children []*jsonUINode  `json:"children"`
}

type jsonStyle struct {
	Direction string          `json:"direction"`
	Align     string          `json:"align"`
	Grow      float32         `json:"grow"`
	Shrink    float32         `json:"shrink"`
	Gap       float32         `json:"gap"`
	Padding   json.RawMessage `json:"padding"`
	Width     float32         `json:"width"`
	Height    float32         `json:"height"`
	WidthPct  float32         `json:"width_pct"`
	HeightPct float32         `json:"height_pct"`
	Rounding  float32         `json:"rounding"`
	Color     json.RawMessage `json:"color"`
}

// LoadUIFile reads and parses a declarative JSON UI template file into a UINode tree.
func LoadUIFile(path string) (*UINode, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("ui: read template %q: %w", path, err)
	}
	return LoadUIJSON(data)
}

// LoadUIJSON parses declarative JSON bytes into a retained UINode hierarchy.
func LoadUIJSON(data []byte) (*UINode, error) {
	var raw jsonUINode
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("ui: invalid template json: %w", err)
	}
	return buildUINode(&raw, nil)
}

func buildUINode(raw *jsonUINode, parent *UINode) (*UINode, error) {
	if raw == nil {
		return nil, nil
	}

	style, err := parseJSONStyle(raw.Style)
	if err != nil {
		return nil, fmt.Errorf("ui: node %q style: %w", raw.ID, err)
	}

	node := &UINode{
		ID:       raw.ID,
		Style:    style,
		Dirty:    true,
		Parent:   parent,
		OnClick:  raw.OnClick,
		OnHover:  raw.OnHover,
		Text:     raw.Text,
		Children: make([]*UINode, 0, len(raw.Children)),
	}

	for _, rawChild := range raw.Children {
		child, err := buildUINode(rawChild, node)
		if err != nil {
			return nil, err
		}
		if child != nil {
			node.Children = append(node.Children, child)
		}
	}

	return node, nil
}

func parseJSONStyle(raw jsonStyle) (LayoutStyle, error) {
	var dir FlexDirection
	switch strings.ToLower(strings.TrimSpace(raw.Direction)) {
	case "", "row":
		dir = FlexRow
	case "column", "col":
		dir = FlexColumn
	default:
		return LayoutStyle{}, fmt.Errorf("unknown flex direction %q", raw.Direction)
	}

	var align AlignItems
	switch strings.ToLower(strings.TrimSpace(raw.Align)) {
	case "", "stretch":
		align = AlignStretch
	case "start", "flex-start":
		align = AlignStart
	case "center":
		align = AlignCenter
	case "end", "flex-end":
		align = AlignEnd
	default:
		return LayoutStyle{}, fmt.Errorf("unknown align %q", raw.Align)
	}

	padding, err := parsePadding(raw.Padding)
	if err != nil {
		return LayoutStyle{}, err
	}

	color, err := parseColor(raw.Color)
	if err != nil {
		return LayoutStyle{}, err
	}

	return LayoutStyle{
		Direction: dir,
		Align:     align,
		Grow:      raw.Grow,
		Shrink:    raw.Shrink,
		Gap:       raw.Gap,
		Padding:   padding,
		Width:     raw.Width,
		Height:    raw.Height,
		WidthPct:  raw.WidthPct,
		HeightPct: raw.HeightPct,
		Rounding:  raw.Rounding,
		Color:     color,
	}, nil
}

func parsePadding(raw json.RawMessage) ([4]float32, error) {
	var pad [4]float32
	if len(raw) == 0 || string(raw) == "null" {
		return pad, nil
	}
	var single float32
	if err := json.Unmarshal(raw, &single); err == nil {
		return [4]float32{single, single, single, single}, nil
	}
	if err := json.Unmarshal(raw, &pad); err == nil {
		return pad, nil
	}
	return pad, fmt.Errorf("padding must be number or [top,right,bottom,left]")
}

// parseColor accepts uint32 (packed ABGR), [r,g,b,a] bytes, or "#RRGGBB"/"#RRGGBBAA" hex strings
// and returns packed ImGui ABGR color (0xAABBGGRR).
func parseColor(raw json.RawMessage) (uint32, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, nil
	}
	var num uint32
	if err := json.Unmarshal(raw, &num); err == nil {
		return num, nil
	}
	var rgba [4]uint8
	if err := json.Unmarshal(raw, &rgba); err == nil {
		return PackABGR(rgba[0], rgba[1], rgba[2], rgba[3]), nil
	}
	var hexStr string
	if err := json.Unmarshal(raw, &hexStr); err == nil {
		s := strings.TrimPrefix(strings.TrimSpace(hexStr), "#")
		if len(s) == 6 {
			s += "FF"
		}
		if len(s) != 8 {
			return 0, fmt.Errorf("invalid hex color %q", hexStr)
		}
		v, perr := strconv.ParseUint(s, 16, 32)
		if perr != nil {
			return 0, fmt.Errorf("invalid hex color %q: %w", hexStr, perr)
		}
		r := uint8((v >> 24) & 0xFF)
		g := uint8((v >> 16) & 0xFF)
		b := uint8((v >> 8) & 0xFF)
		a := uint8(v & 0xFF)
		return PackABGR(r, g, b, a), nil
	}
	return 0, fmt.Errorf("unsupported color format")
}

// PackABGR packs 8-bit RGBA components into Dear ImGui's ImU32 ABGR layout (0xAABBGGRR).
func PackABGR(r, g, b, a uint8) uint32 {
	return uint32(a)<<24 | uint32(b)<<16 | uint32(g)<<8 | uint32(r)
}
