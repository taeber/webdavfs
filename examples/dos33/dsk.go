package dos33

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type sector [256]byte

type diskette struct {
	path     string // Path on host
	name     string
	file     *os.File
	size     int64
	readonly bool
}

func (dsk *diskette) ModTime() (modtime time.Time) {
	if stat, err := dsk.file.Stat(); err == nil {
		modtime = stat.ModTime()
	}
	return
}

func loadDiskette(path string) (dsk diskette, err error) {
	dsk.path = path
	dsk.name = filepath.Base(path)
	dsk.file, err, dsk.readonly = tryOpenFileRW(path)
	if err != nil {
		return
	}

	var fi fs.FileInfo
	fi, err = dsk.file.Stat()
	dsk.size = fi.Size()
	// fmt.Println("DSK", dsk.size, dsk.vtoc)

	return
}

func diskName(dsk *diskette) string { return dsk.name }

/// VTOC

func (dsk *diskette) VTOCFile() (*memFile, error) {
	var vtoc sector
	n, err := dsk.file.ReadAt(vtoc[:], int64(vtocOffset(dsk.size)))
	if err != nil {
		return nil, err
	} else if n != len(vtoc) {
		return nil, fmt.Errorf("partial VTOC read; wanted 0x38 bytes, got %d", n)
	}

	sb := strings.Builder{}
	sb.WriteString("Volume Table of Contents\n")
	sb.WriteString("------------------------\n")
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("  Track of first catalog sector            %3d  $%.2X\n", vtoc[0x1], vtoc[0x1]))
	sb.WriteString(fmt.Sprintf("  Sector of first catalog sector           %3d  $%.2X\n", vtoc[0x2], vtoc[0x2]))
	sb.WriteString(fmt.Sprintf("  DOS version used to INIT diskette        %3d  $%.2X\n", vtoc[0x3], vtoc[0x3]))
	sb.WriteString(fmt.Sprintf("  Diskette volume number                   %3d  $%.2X\n", vtoc[0x6], vtoc[0x6]))
	sb.WriteString(fmt.Sprintf("  Max. Track/Sector pairs in a T/S list    %3d  $%.2X\n", vtoc[0x27], vtoc[0x27]))
	sb.WriteString(fmt.Sprintf("  Last track where sectors were allocated  %3d  $%.2X\n", vtoc[0x30], vtoc[0x30]))
	sb.WriteString(fmt.Sprintf("  Direction of track allocation(+1 or -1)  %+3d  $%.2X\n", int8(vtoc[0x31]), vtoc[0x31]))
	sb.WriteString(fmt.Sprintf("  Tracks per diskette (normally 35)        %3d  $%.2X\n", vtoc[0x34], vtoc[0x34]))
	sb.WriteString(fmt.Sprintf("  Sectors per track (13 or 16)             %3d  $%.2X\n", vtoc[0x35], vtoc[0x35]))
	sb.WriteString(fmt.Sprintf("  Bytes per sector                       %5d  $%.2X%.2X\n", word(vtoc[0x36:0x38]), vtoc[0x37], vtoc[0x36]))
	sb.WriteString("\n")
	sb.WriteString("  Track  Sector (X = used, . = free)\n")
	sb.WriteString("        ")

	// Free Sectors
	//
	// Track   Sector (X = used, . = free)
	//         0 1 2 3 4 5 6 7 8 9 A B C D E F
	//  0 $00  X . . X X X X . . . . . . X X X
	//  1 $01
	// ...
	// 34 $22
	// where 34 is vtoc[(Tracks per diskette)]-1
	// and cols D,E, and F are only used if vtoc[(Sectors per track)] == 16
	cols := int(uint8(vtoc[0x35]))
	if cols > 16 {
		cols = 16
	}

	for i := 0; i < cols; i++ {
		sb.WriteString(fmt.Sprintf(" %X", i))
		if i == 7 {
			sb.WriteRune(' ')
		}
	}
	sb.WriteRune('\n')

	bitmap := 0x38
	rows := int(vtoc[0x34])
	for r := 0; r < rows; r++ {
		sb.WriteString(fmt.Sprintf(" %2d $%.2X ", r, r))
		for c := 0; c < 8; c++ {
			if free := vtoc[bitmap+1] & (0x1 << c); free != 0 {
				sb.WriteString(" .")
			} else {
				sb.WriteString(" X")
			}
		}
		sb.WriteRune(' ')
		for c := 0; c < cols-8; c++ {
			if free := vtoc[bitmap+0] & (0x1 << c); free != 0 {
				sb.WriteString(" .")
			} else {
				sb.WriteString(" X")
			}
		}
		sb.WriteRune('\n')
		bitmap += 4
	}

	return newMemFile("VTOC", sb.String(), dsk.ModTime()), nil
}

func vtocOffset(size int64) uint {
	const (
		D13_SIZE = 116480  // 13 sectors * 256 bytes * 35 tracks
		D13_VTOC = 0xdd00  // 13 sectors * 256 bytes * 17 tracks
		DSK_SIZE = 143360  // 16 sectors * 256 bytes * 35 tracks
		DSK_VTOC = 0x11000 // 16 sectors * 256 bytes * 17 tracks
	)

	if size == D13_SIZE {
		return D13_VTOC
	}

	if size != DSK_SIZE {
		log.Printf("Unexpected size: %d bytes; assuming 16 sectors per track.\n", size)
	}

	return DSK_VTOC
}
