package main

import (
	"flag"
	"fmt"
	"github.com/protomaps/go-pmtiles/pmtiles"
	_ "gocloud.dev/blob/azureblob"
	_ "gocloud.dev/blob/fileblob"
	_ "gocloud.dev/blob/gcsblob"
	_ "gocloud.dev/blob/s3blob"
	"log"
	"net/http"
	"os"
	"runtime/pprof"
	"strconv"
	"time"
	"sort"
)

func main() {
	logger := log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lshortfile)

	if len(os.Args) < 2 {
		helptext := `Usage: pmtiles [COMMAND] [ARGS]

Inspecting pmtiles:
pmtiles show file:// INPUT.pmtiles
pmtiles show "s3://BUCKET_NAME INPUT.pmtiles

Creating pmtiles:
pmtiles convert INPUT.mbtiles OUTPUT.pmtiles
pmtiles convert INPUT_V2.pmtiles OUTPUT_V3.pmtiles

Uploading pmtiles:
pmtiles upload INPUT.pmtiles s3://BUCKET_NAME REMOTE.pmtiles

Running a proxy server:
pmtiles serve "s3://BUCKET_NAME"`
		fmt.Println(helptext)
		os.Exit(1)
	}

	switch os.Args[1] {
	case "extract":
		var z uint8 = 14
		var x_min uint32 = 0
		var x_max uint32 = 10000 // included
		var y_min uint32 = 0
		var y_max uint32 = 10000 // included
		
		var tile_ids []uint64

		for x := x_min; x <= x_max; x++ {
			for y := y_min; y <= y_max; y++ {
				// fmt.Println(z, x, y, pmtiles.ZxyToId(z, x, y))
				tile_ids = append(tile_ids, pmtiles.ZxyToId(z, x, y))
			}
		}

		sort.Slice(tile_ids, func(i, j int) bool { return tile_ids[i] < tile_ids[j]})

		var tile_id_ranges [][2]uint64

		tile_id_ranges = append(tile_id_ranges, [2]uint64{tile_ids[0], tile_ids[0]})

		for i := 1; i < len(tile_ids); i++ {
			if tile_id_ranges[len(tile_id_ranges)-1][1] + 1 == tile_ids[i] {
				tile_id_ranges[len(tile_id_ranges)-1][1] = tile_ids[i]
			} else {
				tile_id_ranges = append(tile_id_ranges, [2]uint64{tile_ids[i], tile_ids[i]})
			}
		}

		fmt.Println(len(tile_ids))
		fmt.Println(len(tile_id_ranges))
		
	case "show":
		err := pmtiles.Show(logger, os.Args[2:])

		if err != nil {
			logger.Fatalf("Failed to show database, %v", err)
		}
	case "serve":
		serveCmd := flag.NewFlagSet("serve", flag.ExitOnError)
		port := serveCmd.String("p", "8080", "port to serve on")
		cors := serveCmd.String("cors", "", "CORS allowed origin value")
		cacheSize := serveCmd.Int("cache", 64, "Cache size in mb")
		serveCmd.Parse(os.Args[2:])
		path := serveCmd.Arg(0)
		if path == "" {
			logger.Println("USAGE: serve  [-p PORT] [-cors VALUE] LOCAL_PATH or https://BUCKET")
			os.Exit(1)
		}
		loop, err := pmtiles.NewLoop(path, logger, *cacheSize, *cors)

		if err != nil {
			logger.Fatalf("Failed to create new loop, %v", err)
		}

		loop.Start()

		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			status_code, headers, body := loop.Get(r.Context(), r.URL.Path)
			for k, v := range headers {
				w.Header().Set(k, v)
			}
			w.WriteHeader(status_code)
			w.Write(body)
			logger.Printf("served %s in %s", r.URL.Path, time.Since(start))
		})

		logger.Printf("Serving %s on HTTP port: %s with Access-Control-Allow-Origin: %s\n", path, *port, *cors)
		logger.Fatal(http.ListenAndServe(":"+*port, nil))
	case "subpyramid":
		subpyramidCmd := flag.NewFlagSet("subpyramid", flag.ExitOnError)
		cpuProfile := subpyramidCmd.Bool("profile", false, "profiling output")
		subpyramidCmd.Parse(os.Args[2:])
		path := subpyramidCmd.Arg(0)
		output := subpyramidCmd.Arg(1)

		var err error
		num_args := make([]int, 5)
		for i := 0; i < 5; i++ {
			if num_args[i], err = strconv.Atoi(subpyramidCmd.Arg(i + 2)); err != nil {
				panic(err)
			}
		}

		if *cpuProfile {
			f, err := os.Create("output.profile")
			if err != nil {
				log.Fatal(err)
			}
			pprof.StartCPUProfile(f)
			defer pprof.StopCPUProfile()
		}
		bounds := "-180,-90,180,90" // TODO deal with antimeridian, center of tile, etc
		pmtiles.SubpyramidXY(logger, path, output, uint8(num_args[0]), uint32(num_args[1]), uint32(num_args[2]), uint32(num_args[3]), uint32(num_args[4]), bounds)
	case "convert":
		convertCmd := flag.NewFlagSet("convert", flag.ExitOnError)
		no_deduplication := convertCmd.Bool("no-deduplication", false, "Don't deduplicate data")
		convertCmd.Parse(os.Args[2:])
		path := convertCmd.Arg(0)
		output := convertCmd.Arg(1)
		err := pmtiles.Convert(logger, path, output, !(*no_deduplication))

		if err != nil {
			logger.Fatalf("Failed to convert %s, %v", path, err)
		}

	case "upload":
		err := pmtiles.Upload(logger, os.Args[2:])

		if err != nil {
			logger.Fatalf("Failed to upload file, %v", err)
		}

	case "validate":
		// pmtiles.Validate()
	default:
		logger.Println("unrecognized command.")
		flag.PrintDefaults()
		os.Exit(1)
	}

}
