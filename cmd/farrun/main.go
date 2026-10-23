package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"
	_ "time/tzdata" // so --tz works on machines without a system zoneinfo

	"github.com/michaszek224/farrun/internal/importer"
	"github.com/michaszek224/farrun/internal/model"
	"github.com/michaszek224/farrun/internal/store"
	"github.com/michaszek224/farrun/internal/web"
)

const (
	defaultDB     = "farrun.db"
	defaultAnonDB = "anon.db"
	defaultAddr   = "127.0.0.1:8080"
)

const usage = `usage: farrun <command> [flags]

commands:
  import   read .tcx files into the database and print a summary
  serve    start the web UI (default http://` + defaultAddr + `)
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, args []string, out, errOut io.Writer) error {
	if len(args) == 0 {
		fmt.Fprint(errOut, usage)
		return errors.New("missing command")
	}

	switch args[0] {
	case "import":
		return runImport(ctx, args[1:], out, errOut)
	case "serve":
		return runServe(ctx, args[1:], out, errOut)
	case "help", "-h", "--help":
		fmt.Fprint(out, usage)
		return nil
	default:
		fmt.Fprint(errOut, usage)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

// commonFlags are the flags every command shares.
type commonFlags struct {
	anon *bool
	db   *string
	tz   *string
}

func addCommonFlags(fs *flag.FlagSet) commonFlags {
	return commonFlags{
		anon: fs.Bool("anon", false, "use the anonymized demo data (anonActivities, "+defaultAnonDB+")"),
		db:   fs.String("db", "", "path to the SQLite database (default "+defaultDB+", or "+defaultAnonDB+" with --anon)"),
		tz:   fs.String("tz", "Europe/Warsaw", "time zone used to show times"),
	}
}

// dbPath picks the database file. Demo data must never end up in the real
// database, so --anon gets its own default.
func dbPath(flagValue string, anon bool) string {
	switch {
	case flagValue != "":
		return flagValue
	case anon:
		return defaultAnonDB
	default:
		return defaultDB
	}
}

// openStore opens the database and returns it with a close function that
// folds a close error into *err, unless an earlier error is already set.
func openStore(path string, err *error) (*store.Store, func(), error) {
	st, openErr := store.Open(path)
	if openErr != nil {
		return nil, nil, openErr
	}
	return st, func() {
		if closeErr := st.Close(); closeErr != nil && *err == nil {
			*err = fmt.Errorf("close database: %w", closeErr)
		}
	}, nil
}

func runImport(ctx context.Context, args []string, out, errOut io.Writer) (err error) {
	fs := flag.NewFlagSet("farrun import", flag.ContinueOnError)
	fs.SetOutput(errOut)
	common := addCommonFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}

	dir := "activities"
	if *common.anon {
		dir = "anonActivities"
	}

	loc, err := time.LoadLocation(*common.tz)
	if err != nil {
		return fmt.Errorf("load time zone %q: %w", *common.tz, err)
	}

	// Check before opening the database, which would create an empty file.
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("read dir %s: %w", dir, err)
	}

	st, closeStore, err := openStore(dbPath(*common.db, *common.anon), &err)
	if err != nil {
		return err
	}
	defer closeStore()

	res, err := importer.Run(ctx, dir, st)
	if err != nil {
		return err
	}

	for _, e := range res.Skipped {
		fmt.Fprintf(errOut, "warning: skipped file: %v\n", e)
	}

	fmt.Fprintf(out, "Imported %d new activities (%d in database", res.Inserted, res.Total)
	if len(res.Skipped) > 0 {
		fmt.Fprintf(out, ", %d files skipped", len(res.Skipped))
	}
	fmt.Fprint(out, ")\n\n")
	for _, a := range res.Activities {
		printActivity(out, a, loc)
	}

	return nil
}

func runServe(ctx context.Context, args []string, out, errOut io.Writer) (err error) {
	fs := flag.NewFlagSet("farrun serve", flag.ContinueOnError)
	fs.SetOutput(errOut)
	common := addCommonFlags(fs)
	addr := fs.String("addr", defaultAddr, "address to listen on")
	if err := fs.Parse(args); err != nil {
		return err
	}

	loc, err := time.LoadLocation(*common.tz)
	if err != nil {
		return fmt.Errorf("load time zone %q: %w", *common.tz, err)
	}

	st, closeStore, err := openStore(dbPath(*common.db, *common.anon), &err)
	if err != nil {
		return err
	}
	defer closeStore()

	handler, err := web.NewHandler(st, loc)
	if err != nil {
		return err
	}

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", *addr, err)
	}
	if !isLoopback(ln.Addr()) {
		fmt.Fprintf(errOut, "warning: listening on %s, so anyone on the network can read your runs and GPS tracks\n", ln.Addr())
	}

	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	fmt.Fprintf(out, "Listening on http://%s (Ctrl+C to stop)\n", ln.Addr())

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()

	select {
	case err := <-serveErr: // Serve only returns on failure here
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shut down server: %w", err)
	}
	return nil
}

func isLoopback(a net.Addr) bool {
	tcp, ok := a.(*net.TCPAddr)
	return ok && tcp.IP.IsLoopback()
}

func printActivity(out io.Writer, a model.Activity, loc *time.Location) {
	secs := int(a.DurationSeconds)
	fmt.Fprintf(out, "%s %s: %.2fkm, %d:%02dm\n",
		a.Sport,
		a.StartTime.In(loc).Format(time.DateOnly),
		a.DistanceMeters/1000,
		secs/60, secs%60)

	fmt.Fprintf(out, "  number of points: %d\n", len(a.Points))
	for i, p := range a.Points {
		if i == 5 {
			break
		}

		gps := "No gps"
		if p.Position != nil {
			gps = fmt.Sprintf("%.5f, %.5f", p.Position.Lat, p.Position.Lon)
		}

		alt, hr, cad := "-", "-", "-"
		if p.AltitudeMeters != nil {
			alt = fmt.Sprintf("%.1f", *p.AltitudeMeters)
		}
		if p.HR != nil {
			hr = fmt.Sprint(*p.HR)
		}
		if p.Cadence != nil {
			cad = fmt.Sprint(*p.Cadence)
		}

		fmt.Fprintf(out, "  #%d %s | gps: %s | wys: %s | hr: %s | kad: %s\n",
			i, p.Time.In(loc).Format(time.TimeOnly), gps, alt, hr, cad)
	}
	fmt.Fprintln(out)
}
