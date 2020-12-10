// Package ldtkgo is a loader for LDtk projects, written in Golang. The general idea is to load a project using ldtkgo.LoadFile() or ldtkgo.LoadBytes(), and then use the resulting Project.
// Generally, the smoothest way to use this in game development seems to be to render the layers out to images, and then draw them onscreen with a rendering or game development
// framework, like Pixel, raylib-go, or ebiten. All of the major elements of LDtk should be supported, including Levels, Layers, Tiles, AutoLayers, IntGrids, Entities, and Properties.
package ldtkgo

import (
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"io/ioutil"
	"math"

	"github.com/tidwall/gjson"
)

//LayerType constants indicating a Layer's type.
const (
	LayerTypeIntGrid  = "IntGrid"
	LayerTypeAutoTile = "AutoLayer"
	LayerTypeTile     = "Tiles"
	LayerTypeEntity   = "Entities"
)

//WorldLayout constants indicating direction or layout system for Worlds.
const (
	WorldLayoutHorizontal = "LinearHorizontal"
	WorldLayoutVertical   = "LinearVertical"
	WorldLayoutFree       = "Free"
	WorldLayoutGridVania  = "GridVania"
)

// Property represents custom Properties created and customized on Entities.
type Property struct {
	Identifier string      `json:"__identifier"`
	Type       string      `json:"__type"`  // The Type of the Property.
	Value      interface{} `json:"__value"` // The value contained within the property.
}

// AsInt returns a property's value as an int. Note that this function doesn't check to ensure the value is the specified type before returning it.
func (p *Property) AsInt() int {
	return int(p.AsFloat64())
}

// AsFloat64 returns a property's value as a float64. Note that this function doesn't check to ensure the value is the specified type before returning it.
func (p *Property) AsFloat64() float64 {
	return p.Value.(float64)
}

// AsString returns a property's value as a string. Can be used for strings, colors, enums, etc. Note that this function doesn't check to ensure the value is the specified type before returning it.
func (p *Property) AsString() string {
	return p.Value.(string)
}

// AsBool returns a property's value as a boolean value. Note that this function doesn't check to ensure the value is the specified type before returning it.
func (p *Property) AsBool() bool {
	return p.Value.(bool)
}

// AsArray returns a property's value as an array of interface{} values. Note that this function doesn't check to ensure the value is the specified type before returning it.
func (p *Property) AsArray() []interface{} {
	return p.Value.([]interface{})
}

// AsColor returns a property's value as a color.Color struct. Note that this function doesn't check to ensure the value is the specified type before returning it.
func (p *Property) AsColor() color.Color {
	color, _ := parseHexColorFast(p.AsString())
	return color
}

// An Entity represents an Entity as placed in the LDtk level.
type Entity struct {
	Identifier    string      `json:"__identifier"` // Name of the Entity
	Position      []int       `json:"px"`           // Position of the Entity (x, y)
	Width, Height int         // Width and height of the Entity definition
	Properties    []*Property `json:"fieldInstances"` // The Properties defined on the Entity
}

// Integer indicates the value for an individual "Integer Object" on the IntGrid layer.
type Integer struct {
	Value    int   // The value of the Integer.
	ID       int   // The ID of the Integer on the IntGrid.
	Position []int // Not actually available from the LDtk file, but added in afterwards as a convenience
}

// Tile represents a graphical tile (whether automatic or manually placed).
type Tile struct {
	Position []int `json:"px"` // Position of the Tile in pixels (x, y)
	Src      []int // The source position on the texture to draw this texture
	Flip     byte  `json:"f"` // Flip bits - first bit is for X-flip, second is for Y. 0 = no flip, 1 = horizontal flip, 2 = vertical flip, 3 = both flipped
	ID       int   `json:"t"` // The ID of the Tile.
}

// Layer represents a Layer, of type either
type Layer struct {
	// The width and height of the layer
	Identifier  string `json:"__identifier"` // Identifier (name) of the Layer
	ID          int
	GridSize    int    `json:"__gridsize"`       // Grid size of the Layer
	OffsetX     int    `json:"__pxTotalOffsetX"` // The offset of the layer
	OffsetY     int    `json:"__pxTotalOffsetY"`
	CellWidth   int    `json:"__cWid"`           // Overall width of the layer in cell count (i.e. a 160x80 level with 16x16 tiles would have a CellWidth and CellHeight of 10x5)
	CellHeight  int    `json:"__cHei"`           // Overall height of the layer in cell count
	Type        string `json:"__type"`           // Type of Layer. Can be compared using LayerType constants
	TilesetPath string `json:"__tilesetRelPath"` // Relative path to the tileset image
	IntGrid     []*Integer
	AutoTiles   []*Tile   `json:"autoLayerTiles"` // Automatically set if IntGrid has values
	Tiles       []*Tile   `json:"gridTiles"`
	Entities    []*Entity `json:"entityInstances"`
}

// ToGridPosition converts the specified position from a position in world space to a position on the Layer's grid. For example, if the layer were 128x128 and had 16x16 tiles, ToGridPosition(32, 16) would return (2, 1).
func (layer *Layer) ToGridPosition(x, y int) (int, int) {
	x /= layer.GridSize
	y /= layer.GridSize
	return x, y
}

// FromGridPosition converts the specified position from a position on the Layer's grid to world space. For example, if the layer were 128x128 and had 16x16 tiles, FromGridPosition(3, 4) would return (48, 64).
func (layer *Layer) FromGridPosition(x, y int) (int, int) {
	x *= layer.GridSize
	y *= layer.GridSize
	return x, y
}

// TileAt returns the Tile at the specified X and Y position (rounded down to the Layer's grid). Note that this doesn't take into account the Layer's local Offset values (so a tile at 16, 16 on a layer with an offset of 64, 64 would still be found at 16, 16).
func (layer *Layer) TileAt(x, y int) *Tile {

	cx, cy := layer.ToGridPosition(x, y)
	for _, tile := range layer.Tiles {
		tx, ty := layer.ToGridPosition(x, y)
		if tx == cx && ty == cy {
			return tile
		}
	}

	return nil

}

// AutoTileAt returns the AutoLayer Tile at the specified X and Y position (rounded down to the Layer's grid). Note that this doesn't take into account the Layer's local Offset values (so a tile at 16, 16 on a layer with an offset of 64, 64 would still be found at 16, 16).
func (layer *Layer) AutoTileAt(x, y int) *Tile {

	cx, cy := layer.ToGridPosition(x, y)
	for _, autoTile := range layer.AutoTiles {
		tx, ty := layer.ToGridPosition(x, y)
		if tx == cx && ty == cy {
			return autoTile
		}
	}

	return nil

}

// IntegerAt returns the IntGrid Integer at the specified X and Y position (rounded down to the Layer's grid). Note that this doesn't take into account the Layer's local Offset values (so a tile at 16, 16 on a layer with an offset of 64, 64 would still be found at 16, 16).
func (layer *Layer) IntegerAt(x, y int) *Integer {

	cx, cy := layer.ToGridPosition(x, y)
	for _, integer := range layer.IntGrid {
		tx, ty := layer.ToGridPosition(x, y)
		if tx == cx && ty == cy {
			return integer
		}
	}

	return nil

}

// Level represents a Level in an LDtk Project.
type Level struct {
	WorldX        int // Position of the Level in the LDtk Project / world
	WorldY        int
	Width         int         `json:"pxWid"` // Width and height of the level in pixels.
	Height        int         `json:"pxHei"`
	Identifier    string      // Name of the Level (i.e. "Level0")
	BGColorString string      `json:"__bgColor"`
	BGColor       color.Color // Background Color for the Level; will automatically default to the Project's if it is left at default in the LDtk project.
	Layers        []*Layer    `json:"layerInstances"` // The layers in the level in the project. Note that layers here (first is "furthest" / at the bottom, last is on top) is reversed compared to LDtk (first is at the top, bottom is on the bottom).
}

// LayerByIdentifier returns a Layer by its identifier (name). Returns nil if the specified Layer isn't found.
func (level *Level) LayerByIdentifier(identifier string) *Layer {
	for _, layer := range level.Layers {
		if layer.Identifier == identifier {
			return layer
		}
	}
	return nil
}

// Project represents a full LDtk Project, allowing you access to the Levels within as well as some project-level properties.
type Project struct {
	WorldLayout     string
	WorldGridWidth  int
	WorldGridHeight int
	BGColorString   string      `json:"defaultLevelBgColor"`
	BGColor         color.Color `json:"-"`
	JSONVersion     string
	Levels          []*Level
	// JSONData    string
}

// LevelAt returns the level that "contains" the point indicated by the X and Y values given, or nil if one isn't found.
func (project *Project) LevelAt(x, y int) *Level {

	for _, level := range project.Levels {

		rect := image.Rect(level.WorldX, level.WorldY, level.WorldX+level.Width, level.WorldY+level.Height)

		if rect.Min.X <= x && rect.Min.Y <= y && rect.Max.X >= x && rect.Max.Y >= y {
			return level
		}

	}

	return nil

}

// LevelByIdentifier returns the level that has the identifier specified, or nil if one isn't found.
func (project *Project) LevelByIdentifier(identifier string) *Level {
	for _, level := range project.Levels {
		if level.Identifier == identifier {
			return level
		}
	}
	return nil
}

// LoadFile loads the LDtk project from the filepath specified. Returns the Project and an error should the loading process fail.
func LoadFile(filepath string) (*Project, error) {

	var project *Project

	var bytes []byte
	var err error

	bytes, err = ioutil.ReadFile(filepath)

	if err == nil {
		project, err = LoadBytes(bytes)
	}

	return project, err

}

// LoadBytes loads the LDtk project using the specified slice of bytes. Returns the Project and an error should the loading process fail.
func LoadBytes(data []byte) (*Project, error) {

	project := &Project{}

	err := json.Unmarshal(data, project)

	dataStr := string(data)

	// Additional convenience fields

	if project.BGColorString != "" {
		project.BGColor, _ = parseHexColorFast(project.BGColorString)
	} else {
		project.BGColor = color.RGBA{}
	}

	for _, level := range project.Levels {

		if level.BGColorString != "" {
			level.BGColor, _ = parseHexColorFast(level.BGColorString)
		} else {
			level.BGColor = color.RGBA{}
		}

		for _, layer := range level.Layers {

			for _, integer := range layer.IntGrid {

				y := int(math.Floor(float64(integer.ID / layer.GridSize)))
				x := integer.ID - y*layer.GridSize
				integer.Position = []int{x, y}

			}

			for _, entity := range layer.Entities {
				for _, entityDef := range gjson.Get(dataStr, `defs.entities`).Array() {
					if entity.Identifier == entityDef.Get("identifier").String() {
						entity.Width = int(entityDef.Get("width").Int())
						entity.Height = int(entityDef.Get("height").Int())
					}
				}
			}

		}

	}

	return project, err

}

// Just straight up cribbing this Hex > Color Conversion Code from StackOverflow: https://stackoverflow.com/questions/54197913/parse-hex-string-to-image-color
// Otherwise, colors from LDtk are just strings that you can't really do anything with.

var errInvalidFormat = errors.New("invalid format")

func parseHexColorFast(s string) (c color.RGBA, err error) {
	c.A = 0xff

	if s[0] != '#' {
		return c, errInvalidFormat
	}

	hexToByte := func(b byte) byte {
		switch {
		case b >= '0' && b <= '9':
			return b - '0'
		case b >= 'a' && b <= 'f':
			return b - 'a' + 10
		case b >= 'A' && b <= 'F':
			return b - 'A' + 10
		}
		err = errInvalidFormat
		return 0
	}

	switch len(s) {
	case 7:
		c.R = hexToByte(s[1])<<4 + hexToByte(s[2])
		c.G = hexToByte(s[3])<<4 + hexToByte(s[4])
		c.B = hexToByte(s[5])<<4 + hexToByte(s[6])
	case 4:
		c.R = hexToByte(s[1]) * 17
		c.G = hexToByte(s[2]) * 17
		c.B = hexToByte(s[3]) * 17
	default:
		err = errInvalidFormat
	}
	return
}