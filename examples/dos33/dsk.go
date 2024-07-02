package dos33

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"time"
)

/// diskette

type diskette struct {
	path     string // Path on host
	name     string
	bytes    []byte
	modTime  time.Time
	size     int64
	readonly bool
	vtoc     []byte
}

func (dsk *diskette) ModTime() time.Time    { return dsk.modTime }
func (dsk *diskette) SectorsPerTrack() uint { return uint(dsk.vtoc[0x35]) }
func (dsk *diskette) Volume() uint          { return uint(dsk.vtoc[0x06]) }

func loadDiskette(path string) (*diskette, error) {
	file, err, readonly := tryOpenFileRW(path)
	if err != nil {
		return nil, err
	}

	var fi fs.FileInfo
	fi, err = file.Stat()
	if err != nil {
		return nil, err
	}

	size := fi.Size()

	buf := make([]byte, size)
	if n, err := file.Read(buf); err != nil {
		if !errors.Is(err, io.EOF) {
			return nil, err
		}
	} else if n != int(size) {
		return nil, fmt.Errorf("failed to read all bytes of %s; wanted %d, got %d", path, size, n)
	}

	return &diskette{
		path:     path,
		name:     filepath.Base(path),
		readonly: readonly,
		size:     fi.Size(),
		modTime:  fi.ModTime(),
		bytes:    buf,
		vtoc:     buf[vtocOffset(size):],
	}, nil
}

func (dsk *diskette) rawSector(track, sector uint) []byte {
	const sectorSize = 256
	offset := (track*dsk.SectorsPerTrack() + sector) * sectorSize
	return dsk.bytes[offset:]
}

func diskName(dsk *diskette) string { return dsk.name }

// Volume Table of Contents
/*
http://fileformats.archiveteam.org/wiki/Apple_DOS_file_system#Volume_Table_Of_Contents

A standard Apple DOS 3.3 has a structure called a Volume Table of Contents
(VTOC) stored at track $11, sector $00

The contents of the VTOC are:

offset
-----
$00    not used
$01    track number of first catalog sector
$02    sector number of first catalog sector
$03    release number of DOS used to INIT this disk
$04-05 not used
$06    Diskette volume number (1-254)
$07-26 not used
$27    maximum number of track/sector pairs which will fit in one file
         track/sector list sector (122 for 256 byte sectors)
$28-2F not used
$30    last track where sectors were allocated
$31    direction of track allocation (+1 or -1)
$32-33 not used
$34    number of tracks per diskette (normally 35)
$35    number of sectors per track (13 or 16)
$36-37 number of bytes per sector (LO/HI format)
$38-3B bit map of free sectors in track 0
$3C-3F bit map of free sectors in track 1
$40-43 bit map of free sectors in track 2
       ...
$BC-BF bit map of free sectors in track 33
$CO-C3 bit map of free sectors in track 34
$C4-FF bit maps for additional tracks if there are more than 35 tracks per
         diskette
*/

func (dsk *diskette) VTOCFile() (*memFile, error) {
	vtoc := dsk.vtoc

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

	return newMemFile("VTOC", sb.String(), dsk.modTime), nil
}

func vtocOffset(size int64) uint {
	const (
		d13Size = 116480  // 13 sectors * 256 bytes * 35 tracks
		d13VTOC = 0xdd00  // 13 sectors * 256 bytes * 17 tracks
		dskSize = 143360  // 16 sectors * 256 bytes * 35 tracks
		dskVTOC = 0x11000 // 16 sectors * 256 bytes * 17 tracks
	)

	if size == d13Size {
		return d13VTOC
	}

	if size == dskSize {
		return dskVTOC
	}

	panic(fmt.Errorf("vtocOffset: unexpected disk size; wanted %d or %d, got %d bytes", d13Size, dskSize, size))
}

/// Catalog
/*
http://fileformats.archiveteam.org/wiki/Apple_DOS_file_system#Catalog

The catalog consists of a 35 byte "File Descriptive Entry" for each file on the disk. The catalog is a chain of sectors, the location of the first Catalog sector is found by looking in the VTOC.

offset
----
$00    Not Used
$01    track number of next catalog sector
$02    sector number of next catalog sector
$03-0A not used
$0B-2D First file descriptive entry
$2E-50 Second file descriptive entry
$51-73 Third file descriptive entry
$74-96 Fourth file descriptive entry
$97-B9 Fifth file descriptive entry
$BA-DC Sixth file descriptive entry
$DD-FF Seventh file descriptive entry
*/

func (dsk *diskette) CATALOGFile() (*memFile, error) {
	sb := strings.Builder{}

	sb.WriteString(fmt.Sprintf("\nDISK VOLUME %d\n\n", dsk.Volume()))

	dsk.Catalog(func(file fileEntry) bool {
		lock := ' '
		if file.IsLocked() {
			lock = '*'
		}
		sb.WriteString(fmt.Sprintf("%c%c %03d ",
			lock,
			file.Type().String()[0],
			file.SectorsUsed()))
		sb.WriteString(fmt.Sprintln(file.Name()))
		return true
	})

	sb.WriteRune('\n')

	return newMemFile("CATALOG", sb.String(), dsk.modTime), nil
}

// Catalog iterates over every file on disk and applies callback, stopping
// iteration when callback returns false.
func (dsk *diskette) Catalog(callback func(fileEntry) bool) {
	const (
		offsetNextTrack  uint = 0x01
		offsetNextSector uint = 0x02
	)

	const (
		offsetFirstEntry = 0x0b
		entrySize        = 35
		maxEntries       = 8
	)

	catalog := dsk.vtoc
	for {
		catalog = dsk.rawSector(uint(catalog[offsetNextTrack]), uint(catalog[offsetNextSector]))
		for i := 0; i < maxEntries; i++ {
			entry := fileEntry(catalog[(offsetFirstEntry + i*entrySize):])
			if entry.IsEmpty() {
				continue
			}

			if !callback(entry) {
				return
			}
		}

		if catalog[offsetNextTrack] == 0 {
			break
		}
	}
}

/// File Descriptive Entry
/*
http://fileformats.archiveteam.org/wiki/Apple_DOS_file_system#File_Descriptive_Entry

offset
----
$00    Track of first track/sector list sector, if this is a deleted file this
        contains FF and the original track number is copied to the last byte of
        the file name (BYTE 20) If this byte contains a 00, the entry is assumed
        to never have been used and is available for use. (This means track 0
        can never be used for data even if the DOS image is 'wiped' from the
        disk)
$01    Sector of first track/sector list sector
$02    File type and flags:
       $80+file type - file is locked
       $00+file type - file is not locked
       $00 - TEXT file
       $01 - INTEGER BASIC file
       $02 - APPLESOFT BASIC file
       $04 - BINARY file
       $08 - S type file
       $10 - RELOCATABLE object module file
       $20 - a type file
       $40 - b type file
$03-20 File Name (30 characters)
$21-22 Length of file in sectors (LO/HI format)
*/

type fileEntry []byte

func (f fileEntry) IsEmpty() bool   { return f[0x00] == 0x00 }
func (f fileEntry) IsDeleted() bool { return f[0x00] == 0xff }
func (f fileEntry) IsLocked() bool  { return f[0x02]&0x80 != 0 }
func (f fileEntry) Type() fileType  { return fileType(f[0x02] & 0x7f) }
func (f fileEntry) Name() string {
	size := 30
	if f.IsDeleted() {
		size--
	}

	sb := strings.Builder{}
	for i := 0; i < size; i++ {
		sb.WriteRune(rune(f[0x03+i] & 0x7f))
	}

	return strings.TrimRight(sb.String(), " ")
}
func (f fileEntry) SectorsUsed() uint16 { return word(f[0x21:0x23]) }

type fileType uint8

const (
	ftText           fileType = 0b0000_0000
	ftIntegerBasic   fileType = 0b0000_0001
	ftApplesoftBasic fileType = 0b0000_0010
	ftBinary         fileType = 0b0000_0100
	ftS              fileType = 0b0000_1000
	ftRelocatable    fileType = 0b0001_0000
	ftA              fileType = 0b0010_0000
	ftB              fileType = 0b0100_0000
)

func (ft fileType) String() string {
	switch ft {
	case ftText:
		return "T"
	case ftIntegerBasic:
		return "I"
	case ftApplesoftBasic:
		return "A"
	case ftBinary:
		return "B"
	case ftS:
		return "S"
	case ftRelocatable:
		return "R"
	case ftA:
		return "A"
	case ftB:
		return "B"
	default:
		panic("filetype.String: switch is non-exhaustive")
	}
}

/// Track Sector List Format
/*
http://fileformats.archiveteam.org/wiki/Apple_DOS_file_system#Track_Sector_List_Format

$00    Not used
$01    Track number of next T/S list of one is needed or zero if no more t/s
        list
$02    Sector number of next T/S list (if one is present)
$03-04 Not used
$05-06 Sector offset in file of the first sector described by this list
$07-0B Not used
$0C-0D Track and sector of first data sector or zeros
$0E-0F Track and sector of second data sector or zeros
$10-FF Up to 120 more track and sector pairs
*/
