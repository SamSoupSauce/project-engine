package tileimporter

import (
	"encoding/xml"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// TMX Flip/Rotation bitmasks (top 3 bits of 32-bit GID)
const (
	FlippedHorizontallyFlag = 0x80000000
	FlippedVerticallyFlag   = 0x40000000
	FlippedDiagonallyFlag   = 0x20000000
	ClearFlagsMask          = 0x1FFFFFFF
)

// Property defines a custom metadata key-value pair in TMX.
type Property struct {
	Name  string `xml:"name,attr"`
	Type  string `xml:"type,attr"`
	Value string `xml:"value,attr"`
	Text  string `xml:",chardata"`
}

func (p Property) GetVal() string {
	if p.Value != "" {
		return p.Value
	}
	return strings.TrimSpace(p.Text)
}

// Layer represents a 2D tile grid layer in a TMX map.
type Layer struct {
	ID        int        `xml:"id,attr"`
	Name      string     `xml:"name,attr"`
	Width     int        `xml:"width,attr"`
	Height    int        `xml:"height,attr"`
	Visible   string     `xml:"visible,attr"`
	Data      Data       `xml:"data"`
	Props     []Property `xml:"properties>property"`
	Tiles     [][]uint32
	FlipFlags [][]uint32
}

func (l *Layer) IsVisible() bool {
	if l.Visible == "0" || strings.ToLower(l.Visible) == "false" {
		return false
	}
	return true
}

func (l *Layer) GetProperty(name, fallback string) string {
	for _, p := range l.Props {
		if strings.EqualFold(p.Name, name) {
			return p.GetVal()
		}
	}
	return fallback
}

// Data holds raw CSV tile grid data.
type Data struct {
	Encoding string `xml:"encoding,attr"`
	Content  string `xml:",chardata"`
}

// Tileset defines tile metadata and properties in TMX.
type Tileset struct {
	FirstGID  uint32        `xml:"firstgid,attr"`
	Name      string        `xml:"name,attr"`
	TileWidth int           `xml:"tilewidth,attr"`
	TileCount int           `xml:"tilecount,attr"`
	Tiles     []TilesetTile `xml:"tile"`
}

// TilesetTile holds per-tile custom properties within a tileset.
type TilesetTile struct {
	ID    uint32     `xml:"id,attr"`
	Props []Property `xml:"properties>property"`
}

// MapObject represents an entity or trigger in an objectgroup.
type MapObject struct {
	ID         int        `xml:"id,attr"`
	Name       string     `xml:"name,attr"`
	Type       string     `xml:"type,attr"`
	X          float32    `xml:"x,attr"`
	Y          float32    `xml:"y,attr"`
	Width      float32    `xml:"width,attr"`
	Height     float32    `xml:"height,attr"`
	Properties []Property `xml:"properties>property"`
}

func (o *MapObject) GetProperty(name, fallback string) string {
	for _, p := range o.Properties {
		if strings.EqualFold(p.Name, name) {
			return p.GetVal()
		}
	}
	return fallback
}

// ObjectGroup groups vector/entity objects in TMX.
type ObjectGroup struct {
	ID      int         `xml:"id,attr"`
	Name    string      `xml:"name,attr"`
	Objects []MapObject `xml:"object"`
}

// Map is the root container of a TMX level file.
type Map struct {
	XMLName      xml.Name      `xml:"map"`
	Width        int           `xml:"width,attr"`
	Height       int           `xml:"height,attr"`
	TileWidth    int           `xml:"tilewidth,attr"`
	TileHeight   int           `xml:"tileheight,attr"`
	Properties   []Property    `xml:"properties>property"`
	Tilesets     []Tileset     `xml:"tileset"`
	Layers       []Layer       `xml:"layer"`
	ObjectGroups []ObjectGroup `xml:"objectgroup"`
}

func (m *Map) GetProperty(name, fallback string) string {
	for _, p := range m.Properties {
		if strings.EqualFold(p.Name, name) {
			return p.GetVal()
		}
	}
	return fallback
}

// ParseTMX parses raw TMX XML bytes and decodes layer tile data.
func ParseTMX(data []byte) (*Map, error) {
	var m Map
	if err := xml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("xml unmarshal error: %w", err)
	}

	for i := range m.Layers {
		layer := &m.Layers[i]
		if strings.ToLower(layer.Data.Encoding) != "csv" {
			continue
		}

		lines := strings.Split(strings.TrimSpace(layer.Data.Content), "\n")
		layer.Tiles = make([][]uint32, 0, len(lines))
		layer.FlipFlags = make([][]uint32, 0, len(lines))

		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.Split(line, ",")
			rowGIDs := make([]uint32, 0, len(parts))
			rowFlags := make([]uint32, 0, len(parts))

			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p == "" {
					continue
				}
				val, err := strconv.ParseUint(p, 10, 32)
				if err != nil {
					continue
				}
				rawGID := uint32(val)
				flags := rawGID & (FlippedHorizontallyFlag | FlippedVerticallyFlag | FlippedDiagonallyFlag)
				cleanGID := rawGID & ClearFlagsMask

				rowGIDs = append(rowGIDs, cleanGID)
				rowFlags = append(rowFlags, flags)
			}

			if len(rowGIDs) > 0 {
				layer.Tiles = append(layer.Tiles, rowGIDs)
				layer.FlipFlags = append(layer.FlipFlags, rowFlags)
			}
		}
	}

	return &m, nil
}

// LoadTMX reads and parses a TMX file from disk.
func LoadTMX(filePath string) (*Map, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file %s: %w", filePath, err)
	}
	return ParseTMX(data)
}
