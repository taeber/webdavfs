package dsk

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Diskette represents an Apple DOS 3.3 formatted disk image.
type Diskette struct {
	path     string // Path on host
	name     string
	bytes    []byte
	modTime  time.Time
	size     int64
	readonly bool
	vtoc     []byte
}

func (dsk *Diskette) Name() string          { return dsk.name }
func (dsk *Diskette) ModTime() time.Time    { return dsk.modTime }
func (dsk *Diskette) SectorsPerTrack() uint { return uint(dsk.vtoc[0x35]) }
func (dsk *Diskette) Volume() uint          { return uint(dsk.vtoc[0x06]) }

// LoadDiskette reads the disk image at path.
func LoadDiskette(path string) (*Diskette, error) {
	file, err, readonly := tryOpenFileRW(path)
	if err != nil {
		return nil, err
	}

	fi, err := file.Stat()
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

	name := filepath.Base(path)
	ext := filepath.Ext(name)

	return &Diskette{
		path:     path,
		name:     name[:len(name)-len(ext)],
		readonly: readonly,
		size:     fi.Size(),
		modTime:  fi.ModTime(),
		bytes:    buf,
		vtoc:     buf[vtocOffset(size):],
	}, nil
}

func (dsk *Diskette) rawSector(track, sector uint) []byte {
	const sectorSize = 256
	offset := (track*dsk.SectorsPerTrack() + sector) * sectorSize
	return dsk.bytes[offset:]
}

/// Volume Table of Contents
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

func (dsk *Diskette) VTOCFile() string {
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

	return sb.String()
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

The catalog consists of a 35 byte "File Descriptive Entry" for each file on the
disk. The catalog is a chain of sectors, the location of the first Catalog
sector is found by looking in the VTOC.

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

// Catalog returns all the files on disk. every file on disk and applies callback, stopping
func (dsk *Diskette) Catalog() (entries []FileEntry) {
	const (
		offsetNextTrack  uint = 0x01
		offsetNextSector uint = 0x02
	)

	entryOffsets := []uint8{0x0B, 0x2E, 0x51, 0x74, 0x97, 0xBA, 0xDD}

	catalog := dsk.vtoc
	for {
		catalog = dsk.rawSector(uint(catalog[offsetNextTrack]), uint(catalog[offsetNextSector]))
		for _, offset := range entryOffsets {
			entry := FileEntry(catalog[offset:])
			if entry.IsEmpty() {
				continue
			}

			entries = append(entries, entry)
		}

		if catalog[offsetNextTrack] == 0 {
			break
		}
	}

	return
}

// writeFileNameln writes out filename to sb, including correctly handling
// INVERSE'd filenames allowable on Apple DOS by using ASCII escape codes.
func writeFileName(sb *strings.Builder, filename string) {
	const (
		escCodeReset   = "\033[0m"
		escCodeInverse = "\033[47;30m"
	)

	inverted := false
	for _, ch := range filename {
		if ch&0x60 == 0 {
			if !inverted {
				sb.WriteString(escCodeInverse)
			}
			inverted = true
			sb.WriteRune(ch | 0x40)
		} else {
			if inverted {
				sb.WriteString(escCodeReset)
			}
			inverted = false
			sb.WriteRune(ch)
		}
	}

	if inverted {
		sb.WriteString(escCodeReset)
	}
}

func RunCatalog(dsk *Diskette) string {
	sb := strings.Builder{}

	sb.WriteString(fmt.Sprintf("\nDISK VOLUME %d\n\n", dsk.Volume()))

	for _, file := range dsk.Catalog() {
		if file.IsDeleted() {
			continue
		}

		lock := ' '
		if file.IsLocked() {
			lock = '*'
		}

		line := fmt.Sprintf("%c%c %03d %s\n",
			lock,
			file.Type().String()[0],
			file.SectorsUsed()%256,
			file.Name().ANSIEscaped())

		sb.WriteString(line)
	}

	sb.WriteRune('\n')

	return sb.String()
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

type FileEntry []byte

func (f FileEntry) IsEmpty() bool   { return f[0x00] == 0x00 }
func (f FileEntry) IsDeleted() bool { return f[0x00] == 0xff }
func (f FileEntry) IsLocked() bool  { return f[0x02]&0x80 != 0 }
func (f FileEntry) Type() FileType  { return FileType(f[0x02] & 0x7f) }
func (f FileEntry) Name() Filename {
	const hiAsciiSpace = 0xA0
	size := 30
	if f.IsDeleted() {
		size--
	}
	for f[0x03+size-1] == hiAsciiSpace {
		size--
	}
	return Filename(f[0x03:][:size])
}
func (f FileEntry) SectorsUsed() uint16 { return word(f[0x21:0x23]) }

// Filename is the name of a DOS 3.3 file.
//
// "DOS 3.x filenames can from 1-30 characters in length, and must start with an
// uppercase letter. They cannot contain commas, colons, but can contain control
// characters."
// https://www.apple2.org/faq/FAQ.dos.prodos.html#DOS_3.x_file_names_and_types
//
// Also, the Apple II has 3 types of text: normal, inverted, and flashing.
// In-memory, a mask is used:
//
//	Inverted = $3F = 0011_1111
//	   Flash = $7F = 0111_1111
//	  Normal = $FF = 1111_1111
//
// "Normal" has the high bit set. This "Hi-ASCII" is incompatible with UTF-8.
//
// DOS supports inverted characters in filenames; modern systems do not.
type Filename []byte

func (name Filename) String() string {
	sb := strings.Builder{}
	for _, ch := range name {
		sb.WriteByte(ch & 0b0111_1111)
	}
	return sb.String()
}

func (name Filename) PathSafe() string {
	sb := strings.Builder{}
	for _, ch := range name {
		isInverted := ch&0b1000_0000 == 0
		if isInverted {
			sb.WriteByte(ch | 0b0100_0000)
		} else {
			sb.WriteByte(ch & 0b0111_1111)
		}
	}
	return sb.String()
}

func (name Filename) ANSIEscaped() string {
	const (
		escCodeReset   = "\033[0m"
		escCodeInverse = "\033[47;30m"
	)

	sb := strings.Builder{}
	prevInverted := false
	for _, ch := range name {
		isInverted := ch&0b1000_0000 == 0
		if isInverted {
			if !prevInverted {
				sb.WriteString(escCodeInverse)
			}
			prevInverted = true
			sb.WriteByte(ch | 0x40)
		} else {
			if prevInverted {
				sb.WriteString(escCodeReset)
			}
			prevInverted = false
			sb.WriteByte(ch & 0b0111_1111)
		}
	}

	if prevInverted {
		sb.WriteString(escCodeReset)
	}

	return sb.String()
}

type FileType uint8

const (
	ftText           FileType = 0b0000_0000
	ftIntegerBasic   FileType = 0b0000_0001
	ftApplesoftBasic FileType = 0b0000_0010
	ftBinary         FileType = 0b0000_0100
	ftS              FileType = 0b0000_1000
	ftRelocatable    FileType = 0b0001_0000
	ftA              FileType = 0b0010_0000
	ftB              FileType = 0b0100_0000
)

func (ft FileType) String() string {
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

/// Helper functions

// tryOpenFileRW tries to open a file for read-write, but falls back to
// read-only if it fails.
func tryOpenFileRW(path string) (file *os.File, err error, readonly bool) {
	file, err = os.OpenFile(path, os.O_RDWR, os.FileMode(0))
	if errors.Is(err, os.ErrPermission) {
		readonly = true
		file, err = os.OpenFile(path, os.O_RDONLY, os.FileMode(0))
	}
	return
}

// word interprets bytes as a little-endian, 16-bit, unsigned integer.
// This is the representation of the MOS 6502.
func word(bytes []byte) uint16 {
	return binary.LittleEndian.Uint16(bytes)
}
