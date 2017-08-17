package command

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/joho/godotenv/autoload"
	"github.com/mitchellh/cli"

	"github.com/nickelchen/wonder/client"
	"github.com/nickelchen/wonder/cmd/wonder/command/render"
	"github.com/nickelchen/wonder/share"
)

var gRow int
var gCol int

func init() {
	gRow, _ = strconv.Atoi(os.Getenv("ROW"))
	gCol, _ = strconv.Atoi(os.Getenv("COL"))
}

type InfoCommand struct {
	Ui cli.Ui

	tiles   [][]share.Tile
	trees   []share.Tree
	flowers []share.Flower
	grass   []share.Grass
}

func (c *InfoCommand) Help() string {
	helpText := `
Usage: wonder info

	Get every information about wonder land. including tiles, sprites etc.
`
	return strings.TrimSpace(helpText)
}

func (c *InfoCommand) Run(args []string) int {
	cmdFlags := flag.NewFlagSet("information", flag.ContinueOnError)
	cmdFlags.Usage = func() { c.Ui.Output(c.Help()) }

	if err := cmdFlags.Parse(args); err != nil {
		return 1
	}

	config := client.Config{
		Addr:    "127.0.0.1:9898",
		Timeout: 20 * time.Second,
	}

	cl, err := client.ClientFromConfig(&config)
	if err != nil {
		c.Ui.Output(fmt.Sprintf("can not get client: %s\n", err))
		return 1
	}

	rend := render.TermRender{Ui: c.Ui}
	// wait group for render to complete
	var rendWg sync.WaitGroup

	respCh1 := make(chan share.InfoResponseObj, 128)
	if err := cl.Info(respCh1); err != nil {
		c.Ui.Output(fmt.Sprintf("can not get info: %s\n", err))
		return 1
	}

	tilesCh := make(chan [][]share.Tile, 128)
	treeCh := make(chan share.Tree, 128)
	flowerCh := make(chan share.Flower, 128)
	grassCh := make(chan share.Grass, 128)

	go c.receiveInfoItems(respCh1, tilesCh, treeCh, flowerCh, grassCh)
	go c.renderInfoItems(&rendWg, &rend, tilesCh, treeCh, flowerCh, grassCh)
	// we need wait for info item process to complete
	rendWg.Add(1)

	respCh2 := make(chan share.EventResponseObj, 128)
	if err := cl.Subscribe(respCh2); err != nil {
		c.Ui.Output(fmt.Sprintf("can not get info: %s\n", err))
		return 1
	}

	moveCh := make(chan share.SpriteMove, 128)
	addCh := make(chan share.SpriteAdd, 128)
	deleteCh := make(chan share.SpriteDelete, 128)

	go c.receiveEventItems(respCh2, moveCh, addCh, deleteCh)
	// we need wait for event item process to complete
	go c.renderEventItems(&rendWg, &rend, moveCh, addCh, deleteCh)
	rendWg.Add(1)

	rendWg.Wait()

	return 0
}

func (c *InfoCommand) receiveInfoItems(
	respCh chan share.InfoResponseObj,
	tilesCh chan [][]share.Tile,
	treeCh chan share.Tree,
	flowerCh chan share.Flower,
	grassCh chan share.Grass) {

	for {
		select {
		// receive from info response
		case r := <-respCh:
			t := r.Type
			p := r.Payload

			c.Ui.Output(fmt.Sprintf("Get Info Response Payload: %v", string(p)))

			switch t {
			case share.InfoItemTypeTile:
				tiles := [][]share.Tile{}
				json.Unmarshal(p, &tiles)
				c.Ui.Output(fmt.Sprintf("receive struct is: %v\n", tiles))

				tilesCh <- tiles

			case share.InfoItemTypeTree:
				spr := share.Tree{}
				json.Unmarshal(p, &spr)
				c.Ui.Output(fmt.Sprintf("receive struct is: %v\n", spr))

				treeCh <- spr

			case share.InfoItemTypeFlower:
				spr := share.Flower{}
				json.Unmarshal(p, &spr)
				c.Ui.Output(fmt.Sprintf("receive struct is: %v\n", spr))

				flowerCh <- spr

			case share.InfoItemTypeGrass:
				spr := share.Grass{}
				json.Unmarshal(p, &spr)
				c.Ui.Output(fmt.Sprintf("receive struct is: %v\n", spr))

				grassCh <- spr

			case share.InfoItemTypeDone:
				c.Ui.Output("received all repsonse. finish")

				return
			}
		}
	}
}

func (c *InfoCommand) receiveEventItems(
	respCh chan share.EventResponseObj,
	moveCh chan share.SpriteMove,
	addCh chan share.SpriteAdd,
	deleteCh chan share.SpriteDelete) {
	for {
		select {
		// receive from subscribe response
		case r := <-respCh:
			t := r.Type
			p := r.Payload

			c.Ui.Output(fmt.Sprintf("Get Subscribed Response Payload: %v", string(p)))

			switch t {
			case share.EventTypeMove:
				event := share.SpriteMove{}
				json.Unmarshal(p, &event)
				c.Ui.Output(fmt.Sprintf("receive sprite move struct is: %v\n", event))

				moveCh <- event

			case share.EventTypeAdd:
				event := share.SpriteAdd{}
				json.Unmarshal(p, &event)
				c.Ui.Output(fmt.Sprintf("receive sprite add struct is: %v\n", event))

				addCh <- event

			case share.EventTypeDelete:
				event := share.SpriteDelete{}
				json.Unmarshal(p, &event)
				c.Ui.Output(fmt.Sprintf("receive sprite delete struct is: %v\n", event))

				deleteCh <- event
			}

		}
	}
}

func (c *InfoCommand) renderInfoItems(wg *sync.WaitGroup,
	rend render.InfoRender,
	tilesCh chan [][]share.Tile,
	treeCh chan share.Tree,
	flowerCh chan share.Flower,
	grassCh chan share.Grass) {

	defer wg.Done()

	rend.Reset(gRow, gCol)

	// 1. wait for tiles to render first
	select {
	case tiles := <-tilesCh:
		for i := 1; i <= len(tiles); i++ {
			tilesRow := tiles[i-1]
			for j := 1; j <= len(tilesRow); j++ {
				t := tilesRow[j-1]
				if t.Gradient > 0 {
					rend.RenderMud(i, j)
				} else {
					rend.RenderGround(i, j)
				}
			}
		}
	}

	// 2. then we can render sprites
	for {
		select {
		case tree := <-treeCh:
			p := tree.GetPoint()
			// point.X and Y is based on 0. but stage is based on 1.
			rend.RenderTree(p.X+1, p.Y+1)
		case flower := <-flowerCh:
			p := flower.GetPoint()
			rend.RenderFlower(p.X+1, p.Y+1)
		case grass := <-grassCh:
			p := grass.GetPoint()
			rend.RenderGrass(p.X+1, p.Y+1)

		// 10 seconds without info received.
		case <-time.After(3 * time.Second):
			c.Ui.Output("no more sprites in 3 seconds, quit rendering info items")
			return
		}
	}
}

func (c *InfoCommand) renderEventItems(wg *sync.WaitGroup,
	rend render.InfoRender,
	moveCh chan share.SpriteMove,
	addCh chan share.SpriteAdd,
	deleteCh chan share.SpriteDelete) {

	defer wg.Done()

	for {
		select {
		case event := <-moveCh:
			c.Ui.Output(fmt.Sprintf("receive sprite move struct is: %v\n", event))
		case event := <-addCh:
			c.Ui.Output(fmt.Sprintf("receive sprite add struct is: %v\n", event))
		case event := <-deleteCh:
			c.Ui.Output(fmt.Sprintf("receive sprite delete struct is: %v\n", event))

		// 10 seconds without event received.
		case <-time.After(10 * time.Second):
			c.Ui.Output("no more events in 10 seconds, quit rendering event items")
			return
		}
	}

}

func (c *InfoCommand) Synopsis() string {
	return "The whole woner land information."
}
