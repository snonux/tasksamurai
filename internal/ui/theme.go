package ui

import (
	"math/rand"
	"strconv"
)

// Theme holds color configuration for the UI.
type Theme struct {
	HeaderFG      string
	SelectedFG    string
	SelectedBG    string
	RowFG         string
	RowBG         string
	StatusFG      string
	StatusBG      string
	StartBG       string
	UltraStartedBG string // background for started tasks in ultra mode
	OverdueBG     string
	PrioLowBG     string
	PrioMedBG     string
	PrioHighBG    string
	SearchFG      string
	SearchBG      string
}

// DefaultTheme returns the color theme used by Task Samurai.
func DefaultTheme() Theme {
	return Theme{
		HeaderFG:   "75",  // steel blue — labels in ultra cards
		SelectedFG: "255", // bright white — text on selected card
		SelectedBG: "238", // dark grey — clean selection highlight on black background
		RowFG:      "0",
		RowBG:      "57",
		StatusFG:   "229", // light yellow
		StatusBG:   "57",  // dark purple — status bar background
		StartBG:        "6",
		UltraStartedBG: "220", // amber yellow — visually distinct "in progress" indicator
		OverdueBG:  "1",
		PrioLowBG:  "28",  // dark green — subtler than bright 10
		PrioMedBG:  "33",  // medium blue — subtler than bright 12
		PrioHighBG: "160", // dark red — subtler than bright 9
		SearchFG:   "16",
		SearchBG:   "220", // amber — easier on eyes than pure yellow 226
	}
}

func RandomTheme() Theme {
	th := Theme{
		HeaderFG:   randColor(),
		SelectedBG: randColor(),
		RowBG:      randColor(),
		StatusBG:   randColor(),
		StartBG:        randColor(),
		UltraStartedBG: randColor(),
		OverdueBG:  randColor(),
		PrioLowBG:  randColor(),
		PrioMedBG:  randColor(),
		PrioHighBG: randColor(),
		SearchBG:   randColor(),
	}
	th.SelectedFG = contrastColor(th.SelectedBG)
	th.RowFG = contrastColor(th.RowBG)
	th.StatusFG = contrastColor(th.StatusBG)
	th.SearchFG = contrastColor(th.SearchBG)
	return th
}

func randColor() string {
	return strconv.Itoa(rand.Intn(256))
}

func contrastColor(bg string) string {
	i, err := strconv.Atoi(bg)
	if err != nil {
		return "0"
	}
	if brightness(i) > 128 {
		return "0"
	}
	return "15"
}

func brightness(i int) float64 {
	r, g, b := xtermRGB(i)
	return 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)
}

func xtermRGB(i int) (int, int, int) {
	if i < 16 {
		var table = [16][3]int{
			{0, 0, 0}, {205, 0, 0}, {0, 205, 0}, {205, 205, 0},
			{0, 0, 238}, {205, 0, 205}, {0, 205, 205}, {229, 229, 229},
			{127, 127, 127}, {255, 0, 0}, {0, 255, 0}, {255, 255, 0},
			{92, 92, 255}, {255, 0, 255}, {0, 255, 255}, {255, 255, 255},
		}
		rgb := table[i]
		return rgb[0], rgb[1], rgb[2]
	}
	if i >= 16 && i <= 231 {
		i -= 16
		r := (i / 36) * 51
		g := (i % 36 / 6) * 51
		b := (i % 6) * 51
		return r, g, b
	}
	v := (i-232)*10 + 8
	return v, v, v
}
