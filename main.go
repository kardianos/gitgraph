package main

import (
	"context"
	"encoding/json"
	"fmt"
	"image/color"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/kardianos/task"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"
)

func main() {
	err := task.Start(context.Background(), time.Second*3, run)
	if err != nil {
		log.Fatal(err)
	}
}

type chart struct {
	Name    string
	Commits []time.Time
}

const (
	dataFilename = "data.js"
	cacheDir     = "cache"
	outputDir    = "output"
)

var loadFrom = filepath.Join(cacheDir, dataFilename)

type FileType map[string]*chart

func (ft FileType) Load(location string) error {
	f, err := os.Open(location)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	d := json.NewDecoder(f)
	store := FileType{}
	err = d.Decode(&store)
	if err != nil {
		return err
	}
	for key, ch := range ft {
		v, ok := store[key]
		if !ok {
			continue
		}
		ch.Commits = v.Commits
	}
	return nil
}
func (ft FileType) Save(location string) error {
	f, err := os.Create(location)
	if err != nil {
		return err
	}
	c := json.NewEncoder(f)
	err = c.Encode(ft)
	cerr := f.Close()
	if err != nil {
		return err
	}
	if cerr != nil {
		return cerr
	}
	return nil
}

var urlLookup = FileType{
	"https://github.com/linuxdeepin/dde-daemon": {
		Name: "DDE Daemon",
	},
	"https://github.com/linuxdeepin/dde-dock": {
		Name: "DDE Dock",
	},
	"https://github.com/linuxdeepin/dde-session-shell": {
		Name: "DDE Session Shell",
	},
}

func run(ctx context.Context) error {
	err := urlLookup.Load(loadFrom)
	if err != nil {
		return err
	}
	updated := false
	for u, ch := range urlLookup {
		if len(ch.Commits) > 0 {
			continue
		}
		fmt.Println("clone", u)
		r, err := git.Clone(memory.NewStorage(), nil, &git.CloneOptions{
			URL: u,
		})
		if err != nil {
			return err
		}
		ref, err := r.Head()
		if err != nil {
			return err
		}
		cIter, err := r.Log(&git.LogOptions{From: ref.Hash()})
		if err != nil {
			return err
		}

		err = cIter.ForEach(func(c *object.Commit) error {
			ch.Commits = append(ch.Commits, c.Committer.When)
			return nil
		})
		if err != nil {
			return err
		}
		updated = true
	}
	if updated {
		err = urlLookup.Save(loadFrom)
		if err != nil {
			return err
		}
	}

	for _, ch := range urlLookup {
		err = display(ch)
		if err != nil {
			return err
		}
	}
	return nil
}

func display(ch *chart) error {
	const GroupSize = 60 * 60 * 24 * 7
	agg := map[int64]int64{}
	now := time.Now()

	for _, dt := range ch.Commits {
		if now.Before(dt) {
			continue
		}
		a := dt.Unix() / GroupSize
		b := a * GroupSize
		agg[b]++
	}
	var maxY float64
	data := make(plotter.XYs, 0, len(agg))
	for x, y := range agg {
		fy := float64(y)
		if fy > maxY {
			maxY = fy
		}
		data = append(data, plotter.XY{
			X: float64(x),
			Y: fy,
		})
	}
	sort.Slice(data, func(i, j int) bool {
		a, b := data[i], data[j]
		return a.X < b.X
	})

	xticks := plot.TimeTicks{
		Ticker: plot.TickerFunc(func(min, max float64) []plot.Tick {
			list := make([]plot.Tick, int((max-min)/GroupSize))
			for i := range list {
				label := ""
				if i%13 == 0 {
					label = "|"
				}
				list[i] = plot.Tick{
					Value: min + (GroupSize * float64(i)),
					Label: label,
				}
			}
			return list
		}),
		Format: "2006-01-02",
	}

	p := plot.New()
	p.Title.Text = ch.Name
	p.X.Tick.Marker = xticks
	p.Y.Label.Text = "Number of Commits (weekly)"
	p.Add(plotter.NewGrid())

	line, points, err := plotter.NewLinePoints(data)
	if err != nil {
		return err
	}
	line.Color = color.RGBA{G: 255, A: 255}
	points.Shape = draw.CircleGlyph{}
	points.Color = color.RGBA{R: 255, A: 255}

	p.Add(line, points)
	p.Y.Max = maxY

	fn := cleanFilename(ch.Name) + ".png"
	err = p.Save(40*vg.Centimeter, 20*vg.Centimeter, filepath.Join(outputDir, fn))
	if err != nil {
		return err
	}
	return nil
}

var cleaner = strings.NewReplacer(
	" ", "_",
	":", "-",
	"\\", "-",
	"/", "-",
)

func cleanFilename(name string) string {
	return cleaner.Replace(name)
}
