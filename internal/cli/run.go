package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/veloben/berichtsheft-cli/internal/api"
	"github.com/veloben/berichtsheft-cli/internal/tui"
)

const defaultBaseURL = "http://127.0.0.1:3847"

type multiString []string

func (m *multiString) String() string { return strings.Join(*m, ",") }
func (m *multiString) Set(v string) error {
	*m = append(*m, strings.TrimSpace(v))
	return nil
}

func Run(args []string) error {
	if len(args) == 0 {
		printHelp()
		return nil
	}

	baseURL := os.Getenv("BERICHTSHEFT_BASE_URL")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	if args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		printHelp()
		return nil
	}

	global := flag.NewFlagSet("berichtsheft-cli", flag.ContinueOnError)
	global.SetOutput(os.Stdout)
	global.StringVar(&baseURL, "base-url", baseURL, "Berichtsheft API base URL")
	if err := global.Parse(args); err != nil {
		return err
	}

	rest := global.Args()
	if len(rest) == 0 {
		printHelp()
		return nil
	}

	client := api.NewClient(baseURL)

	switch rest[0] {
	case "year":
		return runYear(client, rest[1:])
	case "day":
		return runDay(client, rest[1:])
	case "tui":
		return runTUI(client, rest[1:])
	default:
		return fmt.Errorf("unknown command %q", rest[0])
	}
}

func runYear(client *api.Client, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: berichtsheft-cli year <YYYY>")
	}
	year, err := strconv.Atoi(args[0])
	if err != nil {
		return err
	}

	data, err := client.GetYear(year)
	if err != nil {
		return err
	}

	fmt.Printf("Year %d\n", year)
	fmt.Println("KW  Tage  Freigegeben")
	fmt.Println("--  ----  ----------")

	sort.Slice(data.Weeks, func(i, j int) bool {
		wi, _ := strconv.Atoi(data.Weeks[i].Number)
		wj, _ := strconv.Atoi(data.Weeks[j].Number)
		return wi < wj
	})
	for _, w := range data.Weeks {
		approved := 0
		for _, d := range w.Days {
			if d.HasApproval {
				approved++
			}
		}
		wn, _ := strconv.Atoi(w.Number)
		fmt.Printf("%2d  %4d  %10d\n", wn, len(w.Days), approved)
	}

	return nil
}

func runDay(client *api.Client, args []string) error {
	if len(args) < 1 {
		return errors.New("usage: berichtsheft-cli day <get|set> ...")
	}

	switch args[0] {
	case "get":
		return runDayGet(client, args[1:])
	case "set":
		return runDaySet(client, args[1:])
	default:
		return fmt.Errorf("unknown day subcommand %q", args[0])
	}
}

func runDayGet(client *api.Client, args []string) error {
	if len(args) != 3 {
		return errors.New("usage: berichtsheft-cli day get <YYYY> <WW> <D>")
	}
	y, w, d, err := parseYWD(args)
	if err != nil {
		return err
	}

	day, status, err := client.GetDay(y, w, d)
	if err != nil {
		return err
	}
	if status == 404 || day == nil {
		fmt.Println("not found")
		return nil
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(day)
}

func runDaySet(client *api.Client, args []string) error {
	fs := flag.NewFlagSet("day set", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	var status string
	var location string
	var text string
	var timeSpent int
	var setTime bool
	var appendComments multiString
	var replaceComments multiString

	fs.StringVar(&status, "status", "", "Status (anwesend|schulzeit|urlaub|sonstiges)")
	fs.StringVar(&location, "location", "", "Ort (betrieb|schule)")
	fs.StringVar(&text, "text", "", "Markdown Text")
	fs.Var(&appendComments, "comment", "Kommentar anhängen (mehrfach möglich)")
	fs.Var(&replaceComments, "replace-comment", "Kommentare komplett ersetzen (mehrfach möglich)")
	fs.Func("time", "Zeit in Stunden (0..12, ganzzahlig)", func(v string) error {
		i, err := strconv.Atoi(v)
		if err != nil {
			return err
		}
		timeSpent = i
		setTime = true
		return nil
	})

	if err := fs.Parse(args); err != nil {
		return err
	}

	rest := fs.Args()
	if len(rest) != 3 {
		return errors.New("usage: berichtsheft-cli day set [flags] <YYYY> <WW> <D>")
	}
	y, w, d, err := parseYWD(rest)
	if err != nil {
		return err
	}

	cur, statusCode, err := client.GetDay(y, w, d)
	if err != nil {
		return err
	}
	if statusCode == 404 || cur == nil {
		cur = &api.DayData{}
	}
	api.NormalizeDayData(cur)

	if status != "" {
		cur.Metadata.Status = api.NormalizeStatus(status)
	}
	if location != "" {
		cur.Metadata.Location = api.NormalizeLocation(location)
	}
	if text != "" {
		cur.MdData = text
	}
	if setTime {
		cur.Metadata.TimeSpent = api.ClampTime(timeSpent)
	}
	if len(replaceComments) > 0 {
		cur.Metadata.Comments = replaceComments
	}
	if len(appendComments) > 0 {
		cur.Metadata.Comments = append(cur.Metadata.Comments, appendComments...)
	}

	if strings.TrimSpace(cur.MdData) == "" && cur.Metadata.Status != "urlaub" {
		return errors.New("MdData darf nicht leer sein (außer status=urlaub)")
	}

	if err := client.SaveDay(y, w, d, *cur); err != nil {
		return err
	}

	fmt.Printf("saved Y=%d W=%d D=%d\n", y, w, d)
	return nil
}

func runTUI(client *api.Client, args []string) error {
	fs := flag.NewFlagSet("tui", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	year := time.Now().Year()
	week := isoWeek(time.Now())
	fs.IntVar(&year, "year", year, "Jahr")
	fs.IntVar(&week, "week", week, "KW")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return tui.Run(client, year, week)
}

func parseYWD(args []string) (int, int, int, error) {
	y, err := strconv.Atoi(args[0])
	if err != nil {
		return 0, 0, 0, err
	}
	w, err := strconv.Atoi(args[1])
	if err != nil {
		return 0, 0, 0, err
	}
	d, err := strconv.Atoi(args[2])
	if err != nil {
		return 0, 0, 0, err
	}
	if d < 1 || d > 7 {
		return 0, 0, 0, errors.New("day must be 1..7")
	}
	return y, w, d, nil
}

func isoWeek(t time.Time) int {
	_, w := t.ISOWeek()
	return w
}

func printHelp() {
	fmt.Print(`berichtsheft-cli

Usage:
  berichtsheft-cli [--base-url URL] year <YYYY>
  berichtsheft-cli [--base-url URL] day get <YYYY> <WW> <D>
  berichtsheft-cli [--base-url URL] day set [flags] <YYYY> <WW> <D>
  berichtsheft-cli [--base-url URL] tui [--year YYYY --week WW]

Flags for day set:
  --status            anwesend|schulzeit|urlaub|sonstiges
  --location          betrieb|schule
  --time              0..12 (int)
  --text              markdown text
  --comment           append comment (repeatable)
  --replace-comment   replace all comments (repeatable)

Env:
  BERICHTSHEFT_BASE_URL (default http://127.0.0.1:3847)
`)
}
