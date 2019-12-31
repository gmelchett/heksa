package main

import (
	"fmt"
	"github.com/DavidGamba/go-getoptions"
	clr "github.com/logrusorgru/aurora"
	"github.com/raspi/heksa/pkg/iface"
	"github.com/raspi/heksa/pkg/reader"
	"io"
	"os"
	"strconv"
	"strings"
)

var VERSION = `v0.0.0`
var BUILD = `dev`
var BUILDDATE = `0000-00-00T00:00:00+00:00`

const AUTHOR = `Pekka Järvinen`
const HOMEPAGE = `https://github.com/raspi/heksa`

// Parse command line arguments
func getParams() (source iface.ReadSeekerCloser, displays []iface.CharacterFormatter, offsetViewer []iface.OffsetFormatter, limit uint64, startOffset int64, palette [256]clr.Color) {
	opt := getoptions.New()

	opt.HelpSynopsisArgs(`<filename>`)

	opt.Bool(`help`, false,
		opt.Alias("h", "?"),
		opt.Description(`Show this help`),
	)

	opt.Bool(`version`, false,
		opt.Description(`Show version information`),
	)

	argOffset := opt.StringOptional(`offset-format`, `hex`,
		opt.Alias(`o`),
		opt.ArgName(`[fmt1][,fmt2]`),
		opt.Description(`Zero to two of: hex, dec, oct, per, no, ''. First one is displayed on the left side and second one on right after formatters`),
	)

	argFormat := opt.StringOptional(`format`, `hex,asc`,
		opt.Alias(`f`),
		opt.ArgName(`fmt1,fmt2,..`),
		opt.Description(`One or multiple of: hex, dec, oct, bit`),
	)

	argLimit := opt.StringOptional(`limit`, `0`,
		opt.Alias("l"),
		opt.ArgName(`[prefix]bytes`),
		opt.Description(`Read only N bytes (0 = no limit). See NOTES.`),
	)

	argSeek := opt.StringOptional(`seek`, `0`,
		opt.Alias("s"),
		opt.ArgName(`[prefix]offset`),
		opt.Description(`Start reading from certain offset. See NOTES.`),
	)

	remainingArgs, err := opt.Parse(os.Args[1:])

	if opt.Called("help") {
		fmt.Fprintf(os.Stdout, fmt.Sprintf(`heksa - hex file dumper %v - (%v)`+"\n", VERSION, BUILDDATE))
		fmt.Fprintf(os.Stdout, fmt.Sprintf(`(c) %v 2019- [ %v ]`+"\n", AUTHOR, HOMEPAGE))
		fmt.Fprintf(os.Stdout, opt.Help())
		fmt.Fprintf(os.Stdout, fmt.Sprintf(`NOTES:`)+"\n")
		fmt.Fprintf(os.Stdout, fmt.Sprintf(`    - You can use prefixes for seek and limit. 0x = hex, 0b = binary, 0o = octal.`)+"\n")
		fmt.Fprintf(os.Stdout, fmt.Sprintf(`    - Use 'no' or '' for offset formatter for disabling offset output.`)+"\n")
		fmt.Fprintf(os.Stdout, "\n")
		fmt.Fprintf(os.Stdout, fmt.Sprintf(`EXAMPLES:`)+"\n")
		fmt.Fprintf(os.Stdout, fmt.Sprintf(`    heksa -f hex,asc,bit foo.dat`)+"\n")
		fmt.Fprintf(os.Stdout, fmt.Sprintf(`    heksa -o hex,per -f hex,asc foo.dat`)+"\n")
		fmt.Fprintf(os.Stdout, fmt.Sprintf(`    heksa -o hex -f hex,asc,bit foo.dat`)+"\n")
		fmt.Fprintf(os.Stdout, fmt.Sprintf(`    heksa -o no -f bit foo.dat`)+"\n")
		fmt.Fprintf(os.Stdout, fmt.Sprintf(`    heksa -l 0x1024 foo.dat`)+"\n")
		fmt.Fprintf(os.Stdout, fmt.Sprintf(`    heksa -s 0b1010 foo.dat`)+"\n")
		os.Exit(0)
	} else if opt.Called("version") {
		fmt.Fprintf(os.Stdout, fmt.Sprintf(`%v build %v on %v`+"\n", VERSION, BUILD, BUILDDATE))
		os.Exit(0)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %s\n\n", err)
		fmt.Fprintf(os.Stderr, opt.Help(getoptions.HelpSynopsis))
		os.Exit(1)
	}

	limit, err = strconv.ParseUint(*argLimit, 0, 64)
	if err != nil {
		fmt.Fprintln(os.Stderr, fmt.Sprintf(`error parsing limit: %v`, err))
		os.Exit(1)
	}

	startOffset, err = strconv.ParseInt(*argSeek, 0, 64)
	if err != nil {
		fmt.Fprintln(os.Stderr, fmt.Sprintf(`error parsing seek: %v`, err))
		os.Exit(1)
	}

	offsetViewer, err = reader.GetOffsetFormatters(strings.Split(*argOffset, `,`))
	if err != nil {
		fmt.Fprintln(os.Stderr, fmt.Sprintf(`error getting offset formatter: %v`, err))
		os.Exit(1)
	}

	displays, err = reader.GetViewers(strings.Split(*argFormat, `,`))
	if err != nil {
		fmt.Fprintln(os.Stderr, fmt.Sprintf(`error getting formatter: %v`, err))
		os.Exit(1)
	}

	// Initialize palette
	for i := uint8(0); i < 255; i++ {
		color, ok := defaultCharacterColors[i]
		if !ok {
			// Fall back
			color = defaultColor
		}

		palette[i] = color
	}

	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		// Stdin has data
		source = os.Stdin

		// No clue of file size when streaming from stdin
		for idx, _ := range offsetViewer {
			offsetViewer[idx].SetFileSize(0)
		}
	} else {
		// Read file
		if len(remainingArgs) != 1 {
			fmt.Fprintln(os.Stderr, fmt.Sprintf(`error: no file given as argument, see --help`))
			os.Exit(1)
		}

		fpath := remainingArgs[0]

		fhandle, err := os.Open(fpath)
		if err != nil {
			fmt.Fprintln(os.Stderr, fmt.Sprintf(`error opening file: %v`, err))
			os.Exit(1)
		}

		fi, err := fhandle.Stat()
		if err != nil {
			fmt.Fprintln(os.Stderr, fmt.Sprintf(`error stat'ing file: %v`, err))
			os.Exit(1)
		}

		if fi.IsDir() {
			fmt.Fprintln(os.Stderr, fmt.Sprintf(`error: %v is directory`, fpath))
			os.Exit(1)
		}

		// Hint offset viewer
		for idx, _ := range offsetViewer {
			offsetViewer[idx].SetFileSize(fi.Size())
		}

		source = fhandle

	}

	return source, displays, offsetViewer, limit, startOffset, palette
}

func main() {
	source, displays, offViewer, limit, startOffset, palette := getParams()

	if startOffset != 0 {
		// Seek to given offset

		_, err := source.Seek(startOffset, io.SeekCurrent)
		if err != nil {
			fmt.Fprintln(os.Stderr, fmt.Sprintf(`couldn't seek: %v`, err))
			os.Exit(1)
		}
	}

	r := reader.New(source, offViewer, displays, palette)

	// Dump hex
	for {
		s, err := r.Read()
		if err != nil {
			if err == io.EOF {
				break
			}

			fmt.Fprintln(os.Stderr, fmt.Sprintf(`error while reading file: %v`, err))
			os.Exit(1)
		}

		fmt.Println(s)

		if limit > 0 && r.ReadBytes >= limit {
			// Limit is set and found
			break
		}

	}

	source.Close()

}
