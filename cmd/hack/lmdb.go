package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/ledgerwatch/turbo-geth/common/dbutils"
	"github.com/ledgerwatch/turbo-geth/ethdb"
	"github.com/ledgerwatch/turbo-geth/log"
)

const PageSize = 4096
const MdbMagic uint32 = 0xBEEFC0DE
const MdbDataVersion uint32 = 1

const BranchPageFlag uint16 = 1
const LeafPageFlag uint16 = 2
const OverflowPageFlag uint16 = 4

const BigDataFlag uint16 = 1
const HeaderSize int = 16

// Generates an empty database and returns the file name
func generate1(_ ethdb.Tx) error {
	return nil
}

// Generates a database with single table and two key-value pair in "t" DBI, and returns the file name
func generate2(tx ethdb.Tx, entries int) error {
	if err := tx.(ethdb.BucketMigrator).CreateBucket("t"); err != nil {
		return err
	}
	c := tx.Cursor("t")
	defer c.Close()
	for i := 0; i < entries; i++ {
		k := fmt.Sprintf("%05d", i)
		if err := c.Append([]byte(k), []byte("very_short_value")); err != nil {
			return err
		}
	}
	return nil
}

// Generates a database with 100 (maximum) of DBIs to produce branches in MAIN_DBI
func generate3(tx ethdb.Tx) error {
	for i := 0; i < 61; i++ {
		k := fmt.Sprintf("table_%05d", i)
		if err := tx.(ethdb.BucketMigrator).CreateBucket(k); err != nil {
			return err
		}
	}
	return nil
}

// Generates a database with one table, containing 1 short and 1 long (more than one page) values
func generate4(tx ethdb.Tx) error {
	if err := tx.(ethdb.BucketMigrator).CreateBucket("t"); err != nil {
		return err
	}
	c := tx.Cursor("t")
	defer c.Close()
	if err := c.Append([]byte("k1"), []byte("very_short_value")); err != nil {
		return err
	}
	if err := c.Append([]byte("k2"), []byte(strings.Repeat("long_value_", 1000))); err != nil {
		return err
	}
	return nil
}

func dot2png(dotFileName string) string {
	return strings.TrimSuffix(dotFileName, filepath.Ext(dotFileName)) + ".png"
}

func defragSteps(filename string, generateF func(ethdb.Tx) error) error {
	dir, err := ioutil.TempDir(".", "lmdb-vis")
	if err != nil {
		return fmt.Errorf("creating temp dir for lmdb visualisation: %w", err)
	}
	defer os.RemoveAll(dir)
	var kv ethdb.KV
	kv, err = ethdb.NewLMDB().Path(dir).WithBucketsConfig(func(dbutils.BucketsCfg) dbutils.BucketsCfg {
		return make(dbutils.BucketsCfg)
	}).Open()
	if err != nil {
		return fmt.Errorf("opening LMDB database: %w", err)
	}
	defer kv.Close()
	if err = kv.Update(context.Background(), generateF); err != nil {
		return fmt.Errorf("generating data in temp db: %w", err)
	}
	if err != nil {
		return fmt.Errorf("generate db for %s: %w", filename, err)
	}
	var f *os.File
	if f, err = os.Create(filename); err != nil {
		return fmt.Errorf("open %s: %w", filename, err)
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()
	if err = textInfo(dir, w); err != nil {
		return fmt.Errorf("textInfo for %s: %w", filename, err)
	}
	if err = w.Flush(); err != nil {
		return fmt.Errorf("flush %s: %w", filename, err)
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", filename, err)
	}
	//nolint:gosec
	cmd := exec.Command("dot", "-Tpng:gd", "-o", dot2png(filename), filename)
	var output []byte
	if output, err = cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("dot generation error: %w, output: %sn", err, output)
	}
	return nil
}

func defrag() error {
	if err := defragSteps("vis1.dot", generate1); err != nil {
		return err
	}
	if err := defragSteps("vis2.dot", func(tx ethdb.Tx) error { return generate2(tx, 2) }); err != nil {
		return err
	}
	if err := defragSteps("vis3.dot", generate3); err != nil {
		return err
	}
	if err := defragSteps("vis4.dot", func(tx ethdb.Tx) error { return generate2(tx, 200) }); err != nil {
		return err
	}
	if err := defragSteps("vis5.dot", func(tx ethdb.Tx) error { return generate4(tx) }); err != nil {
		return err
	}
	return nil
}

func textInfo(chaindata string, visStream io.Writer) error {
	log.Info("Text Info", "db", chaindata)
	fmt.Fprintf(visStream, "digraph lmdb {\nrankdir=LR\n")
	datafile := path.Join(chaindata, "data.mdb")
	f, err := os.Open(datafile)
	if err != nil {
		return fmt.Errorf("opening data.mdb: %v", err)
	}
	defer f.Close()
	var meta [PageSize]byte
	// Read meta page 0
	if _, err = f.ReadAt(meta[:], 0*PageSize); err != nil {
		return fmt.Errorf("reading meta page 0: %v", err)
	}
	pos, pageID, _, _ := readPageHeader(meta[:], 0)
	if pageID != 0 {
		return fmt.Errorf("meta page 0 has wrong page ID: %d != %d", pageID, 0)
	}
	var freeRoot0, mainRoot0, txnID0 uint64
	var freeDepth0, mainDepth0 uint16
	freeRoot0, freeDepth0, mainRoot0, mainDepth0, txnID0, err = readMetaPage(meta[:], pos)
	if err != nil {
		return fmt.Errorf("reading meta page 0: %v", err)
	}

	// Read meta page 0
	if _, err = f.ReadAt(meta[:], 1*PageSize); err != nil {
		return fmt.Errorf("reading meta page 1: %v", err)
	}
	pos, pageID, _, _ = readPageHeader(meta[:], 0)
	if pageID != 1 {
		return fmt.Errorf("meta page 1 has wrong page ID: %d != %d", pageID, 1)
	}
	var freeRoot1, mainRoot1, txnID1 uint64
	var freeDepth1, mainDepth1 uint16
	freeRoot1, freeDepth1, mainRoot1, mainDepth1, txnID1, err = readMetaPage(meta[:], pos)
	if err != nil {
		return fmt.Errorf("reading meta page 1: %v", err)
	}

	var freeRoot, mainRoot uint64
	var freeDepth, mainDepth uint16
	if txnID0 > txnID1 {
		freeRoot = freeRoot0
		freeDepth = freeDepth0
		mainRoot = mainRoot0
		mainDepth = mainDepth0
	} else {
		freeRoot = freeRoot1
		freeDepth = freeDepth1
		mainRoot = mainRoot1
		mainDepth = mainDepth1
	}
	log.Info("FREE_DBI", "root page ID", freeRoot, "depth", freeDepth)
	log.Info("MAIN_DBI", "root page ID", mainRoot, "depth", mainDepth)

	fmt.Fprintf(visStream, "meta [shape=Mrecord style=filled fillcolor=\"#AED6F1\" label=\"<free_root>FREE_DBI")
	if freeRoot != 0xffffffffffffffff {
		fmt.Fprintf(visStream, "=%d", freeRoot)
	}
	fmt.Fprintf(visStream, "|<main_root>MAIN_DBI")
	if mainRoot != 0xffffffffffffffff {
		fmt.Fprintf(visStream, "=%d", mainRoot)
	}
	fmt.Fprintf(visStream, "\"];\n")

	var freelist = make(map[uint64]bool)
	if freeRoot == 0xffffffffffffffff {
		log.Info("empty freelist")
	} else {
		if freelist, err = readFreelist(f, freeRoot, freeDepth); err != nil {
			return err
		}
		fmt.Fprintf(visStream, "meta:free_root -> p_%d;\n", freeRoot)
	}
	var maintree = make(map[uint64]struct{})
	if mainRoot == 0xffffffffffffffff {
		log.Info("empty maintree")
	} else {
		if maintree, err = readMainTree(f, mainRoot, mainDepth, visStream); err != nil {
			return err
		}
		fmt.Fprintf(visStream, "meta:main_root -> p_%d;\n", mainRoot)
	}

	// Now scan all non meta and non-freelist pages
	pageID = 2
	_, mainOk := maintree[pageID]
	for _, ok := freelist[pageID]; ok || mainOk; _, ok = freelist[pageID] {
		pageID++
		_, mainOk = maintree[pageID]
	}
	count := 0
	for _, err = f.ReadAt(meta[:], int64(pageID)*PageSize); err == nil; _, err = f.ReadAt(meta[:], int64(pageID)*PageSize) {
		var pageNum int
		if pageNum, err = scanPage(meta[:], visStream); err != nil {
			return err
		}
		count += pageNum
		if count%(1024*256) == 0 {
			log.Info("Scaned", "Gb", count/(1024*256))
		}
		pageID += uint64(pageNum)
		_, mainOk = maintree[pageID]
		for _, ok := freelist[pageID]; ok || mainOk; _, ok = freelist[pageID] {
			pageID++
			_, mainOk = maintree[pageID]
		}
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	log.Info("Scaned", "pages", count, "page after last", pageID)
	fmt.Fprintf(visStream, "}\n")
	return nil
}

func scanPage(page []byte, visStream io.Writer) (int, error) {
	pos, pageID, flags, lowerFree := readPageHeader(page, 0)
	if flags&LeafPageFlag != 0 {
		num := (lowerFree - pos) / 2
		interesting := make(map[int]struct{}) // Lines that have something interesting in them
		// First pass - figuring out interesting lines
		interesting[0] = struct{}{}
		interesting[1] = struct{}{}
		interesting[num-1] = struct{}{}
		interesting[num-2] = struct{}{}
		for i := 0; i < num; i++ {
			nodePtr := int(binary.LittleEndian.Uint16(page[HeaderSize+i*2:]))
			nodeFlags := binary.LittleEndian.Uint16(page[nodePtr+4:])
			keySize := int(binary.LittleEndian.Uint16(page[nodePtr+6:]))
			if nodeFlags&BigDataFlag > 0 {
				interesting[i-1] = struct{}{}
				interesting[i] = struct{}{}
				interesting[i+1] = struct{}{}
				overflowPageID := binary.LittleEndian.Uint64(page[nodePtr+8+keySize:])
				fmt.Fprintf(visStream, "p_%d:n%d -> p_%d;\n", pageID, i, overflowPageID)
			}
		}
		fmt.Fprintf(visStream, "p_%d [shape=record style=filled fillcolor=\"#D5F5E3\" label=\"", pageID)
		for i := 0; i < num; i++ {
			_, i0 := interesting[i]
			_, i1 := interesting[i+1]
			if !i0 && !i1 {
				continue
			}
			if !i0 {
				count := 0
				for j := i; !i0; _, i0 = interesting[j] {
					count++
					j--
				}
				fmt.Fprintf(visStream, "|... %d more records here ...", count)
				continue
			}
			if i > 0 {
				fmt.Fprintf(visStream, "|")
			}
			nodePtr := int(binary.LittleEndian.Uint16(page[HeaderSize+i*2:]))
			dataSize := int(binary.LittleEndian.Uint32(page[nodePtr:]))
			nodeFlags := binary.LittleEndian.Uint16(page[nodePtr+4:])
			keySize := int(binary.LittleEndian.Uint16(page[nodePtr+6:]))
			key := string(page[nodePtr+8 : nodePtr+8+keySize])
			if nodeFlags&BigDataFlag > 0 {
				overflowPageID := binary.LittleEndian.Uint64(page[nodePtr+8+keySize:])
				fmt.Fprintf(visStream, "<n%d>%s:%d", i, key, overflowPageID)
			} else {
				val := string(page[nodePtr+8+keySize : nodePtr+8+keySize+dataSize])
				fmt.Fprintf(visStream, "%s:%s", key, val)
			}
		}
		fmt.Fprintf(visStream, "\"];\n")
	} else if flags&BranchPageFlag != 0 {
		num := (lowerFree - pos) / 2
		fmt.Fprintf(visStream, "p_%d [shape=record style=filled fillcolor=\"#F6DDCC\" label=\"", pageID)
		for i := 0; i < num; i++ {
			if i > 0 {
				fmt.Fprintf(visStream, "|")
			}
			nodePtr := int(binary.LittleEndian.Uint16(page[HeaderSize+i*2:]))
			pagePtr := binary.LittleEndian.Uint64(page[nodePtr:]) & 0xFFFFFFFFFFFF
			fmt.Fprintf(visStream, "<n%d>%d", i, pagePtr)
		}
		fmt.Fprintf(visStream, "\"];\n")
		for i := 0; i < num; i++ {
			nodePtr := int(binary.LittleEndian.Uint16(page[HeaderSize+i*2:]))
			pagePtr := binary.LittleEndian.Uint64(page[nodePtr:]) & 0xFFFFFFFFFFFF
			fmt.Fprintf(visStream, "p_%d:n%d -> p_%d;\n", pageID, i, pagePtr)
		}
	} else if flags&OverflowPageFlag != 0 {
		_, _, overflowNum := readOverflowPageHeader(page[:], 0)
		fmt.Fprintf(visStream, "p_%d [shape=record style=filled fillcolor=\"#D5F5E3\" label=\"Overflow %d pages\"];", pageID, overflowNum)
		return overflowNum, nil
	} else {
		return 0, fmt.Errorf("unimplemented processing for page type, flags: %d", flags)
	}
	return 1, nil
}

func readMainTree(f io.ReaderAt, mainRoot uint64, mainDepth uint16, visStream io.Writer) (map[uint64]struct{}, error) {
	var maintree = make(map[uint64]struct{})
	var mainEntries int
	var pages [8][PageSize]byte // Stack of pages
	var pageIDs [8]uint64
	var numKeys [8]int
	var indices [8]int
	var visbufs [8]strings.Builder
	var top int
	var pos int
	var pageID uint64
	pageIDs[0] = mainRoot
	for top >= 0 {
		branch := top < int(mainDepth)-1
		i := indices[top]
		num := numKeys[top]
		page := &pages[top]
		pageID = pageIDs[top]
		if num == 0 {
			maintree[pageID] = struct{}{}
			if _, err := f.ReadAt(page[:], int64(pageID*PageSize)); err != nil {
				return nil, fmt.Errorf("reading FREE_DBI page: %v", err)
			}
			var flags uint16
			var lowerFree int
			pos, _, flags, lowerFree = readPageHeader(page[:], 0)
			branchFlag := flags&BranchPageFlag > 0
			if branchFlag && !branch {
				return nil, fmt.Errorf("unexpected branch page on level %d of FREE_DBI", top)
			}
			if !branchFlag && branch {
				return nil, fmt.Errorf("expected branch page on level %d of FREE_DBI", top)
			}
			num = (lowerFree - pos) / 2
			i = 0
			numKeys[top] = num
			indices[top] = i
			visbufs[top].Reset()
			if branch {
				fmt.Fprintf(&visbufs[top], "p_%d [shape=record style=filled fillcolor=\"#F6DDCC\" label=\"", pageID)
			} else {
				fmt.Fprintf(&visbufs[top], "p_%d [shape=record style=filled fillcolor=\"#D5F5E3\" label=\"", pageID)
			}
		} else if i < num {
			nodePtr := int(binary.LittleEndian.Uint16(page[HeaderSize+i*2:]))
			indices[top] = i + 1
			if branch {
				if i > 0 {
					fmt.Fprintf(&visbufs[top], "|")
				}
				pagePtr := binary.LittleEndian.Uint64(page[nodePtr:]) & 0xFFFFFFFFFFFF
				fmt.Fprintf(&visbufs[top], "<n%d>%d", i, pagePtr)
				fmt.Fprintf(visStream, "p_%d:n%d -> p_%d;\n", pageID, i, pagePtr)
				top++
				indices[top] = 0
				numKeys[top] = 0
				pageIDs[top] = pagePtr
			} else {
				if i > 0 {
					fmt.Fprintf(&visbufs[top], "|")
				}
				mainEntries++
				dataSize := int(binary.LittleEndian.Uint32(page[nodePtr:]))
				flags := binary.LittleEndian.Uint16(page[nodePtr+4:])
				keySize := int(binary.LittleEndian.Uint16(page[nodePtr+6:]))
				if flags&BigDataFlag > 0 {
					return nil, fmt.Errorf("unexpected overflow pages")
				}
				if dataSize != 48 {
					return nil, fmt.Errorf("expected datasize 48, got: %d", dataSize)
				}
				tableName := string(page[nodePtr+8 : nodePtr+8+keySize])
				pagePtr := binary.LittleEndian.Uint64(page[nodePtr+8+keySize+40:])
				if pagePtr != 0xffffffffffffffff {
					fmt.Fprintf(&visbufs[top], "<n%d>%s=%d", i, tableName, pagePtr)
					fmt.Fprintf(visStream, "p_%d:n%d -> p_%d;\n", pageID, i, pagePtr)
				} else {
					fmt.Fprintf(&visbufs[top], "%s", tableName)
				}
				//fmt.Printf("Table: %s, root page: %d\n", page[nodePtr+8:nodePtr+8+keySize], binary.LittleEndian.Uint64(page[nodePtr+8+keySize+40:]))
			}
			i++
		} else {
			fmt.Fprintf(&visbufs[top], "\"];\n")
			if _, err := visStream.Write([]byte(visbufs[top].String())); err != nil {
				return nil, fmt.Errorf("writing buffer into main stream: %w", err)
			}
			top--
		}
	}
	log.Info("Main tree", "entries", mainEntries)
	return maintree, nil
}

// Returns a map of pageIDs to bool. If value is true, this page is free. If value is false,
// this page is a part of freelist structure itself
func readFreelist(f io.ReaderAt, freeRoot uint64, freeDepth uint16) (map[uint64]bool, error) {
	var freelist = make(map[uint64]bool)
	var freepages int
	var freeEntries int
	var overflows int
	var pages [8][PageSize]byte // Stack of pages
	var pageIDs [8]uint64
	var numKeys [8]int
	var indices [8]int
	var top int
	var pos int
	var pageID uint64
	var overflow [PageSize]byte
	pageIDs[0] = freeRoot
	for top >= 0 {
		branch := top < int(freeDepth)-1
		i := indices[top]
		num := numKeys[top]
		page := &pages[top]
		if num == 0 {
			pageID = pageIDs[top]
			freelist[pageID] = false
			if _, err := f.ReadAt(page[:], int64(pageID*PageSize)); err != nil {
				return nil, fmt.Errorf("reading FREE_DBI page: %v", err)
			}
			var flags uint16
			var lowerFree int
			pos, _, flags, lowerFree = readPageHeader(page[:], 0)
			branchFlag := flags&BranchPageFlag > 0
			if branchFlag && !branch {
				return nil, fmt.Errorf("unexpected branch page on level %d of FREE_DBI", top)
			}
			if !branchFlag && branch {
				return nil, fmt.Errorf("expected branch page on level %d of FREE_DBI", top)
			}
			num = (lowerFree - pos) / 2
			i = 0
			numKeys[top] = num
			indices[top] = i
		} else if i < num {
			nodePtr := int(binary.LittleEndian.Uint16(page[HeaderSize+i*2:]))
			i++
			indices[top] = i
			if branch {
				top++
				indices[top] = 0
				numKeys[top] = 0
				pageIDs[top] = binary.LittleEndian.Uint64(page[nodePtr:]) & 0xFFFFFFFFFFFF
			} else {
				freeEntries++
				dataSize := int(binary.LittleEndian.Uint32(page[nodePtr:]))
				flags := binary.LittleEndian.Uint16(page[nodePtr+4:])
				keySize := int(binary.LittleEndian.Uint16(page[nodePtr+6:]))
				if flags&BigDataFlag > 0 {
					overflowPageID := binary.LittleEndian.Uint64(page[nodePtr+8+keySize:])
					freelist[overflowPageID] = false
					if _, err := f.ReadAt(overflow[:], int64(overflowPageID*PageSize)); err != nil {
						return nil, fmt.Errorf("reading FREE_DBI overflow page: %v", err)
					}
					var overflowNum int
					_, _, overflowNum = readOverflowPageHeader(overflow[:], 0)
					overflows += overflowNum
					left := dataSize - 8
					// Start with pos + 8 because first 8 bytes is the size of the list
					for j := HeaderSize + 8; j < PageSize && left > 0; j += 8 {
						pn := binary.LittleEndian.Uint64(overflow[j:])
						freepages++
						freelist[pn] = true
						left -= 8
					}
					for i := 1; i < overflowNum; i++ {
						freelist[overflowPageID+uint64(i)] = false
						if _, err := f.ReadAt(overflow[:], int64((overflowPageID+uint64(i))*PageSize)); err != nil {
							return nil, fmt.Errorf("reading FREE_DBI overflow page: %v", err)
						}
						for j := 0; j < PageSize && left > 0; j += 8 {
							pn := binary.LittleEndian.Uint64(overflow[j:])
							freepages++
							freelist[pn] = true
							left -= 8
						}
					}
				} else {
					// First 8 bytes is the size of the list
					for j := nodePtr + 8 + keySize + 8; j < nodePtr+8+keySize+dataSize; j += 8 {
						pn := binary.LittleEndian.Uint64(page[j:])
						freepages++
						freelist[pn] = true
					}
				}
			}
		} else {
			top--
		}
	}
	log.Info("Freelist", "pages", freepages, "entries", freeEntries, "overflows", overflows)
	return freelist, nil
}

func readPageHeader(page []byte, pos int) (newpos int, pageID uint64, flags uint16, lowerFree int) {
	pageID = binary.LittleEndian.Uint64(page[pos:])
	pos += 8
	pos += 2 // Padding
	flags = binary.LittleEndian.Uint16(page[pos:])
	pos += 2
	lowerFree = int(binary.LittleEndian.Uint16(page[pos:]))
	pos += 4 // Overflow page number / lower upper bound of free space
	newpos = pos
	return
}

func readMetaPage(page []byte, pos int) (freeRoot uint64, freeDepth uint16, mainRoot uint64, mainDepth uint16, txnID uint64, err error) {
	magic := binary.LittleEndian.Uint32(page[pos:])
	if magic != MdbMagic {
		err = fmt.Errorf("meta page has wrong magic: %X != %X", magic, MdbMagic)
		return
	}
	pos += 4
	version := binary.LittleEndian.Uint32(page[pos:])
	if version != MdbDataVersion {
		err = fmt.Errorf("meta page has wrong version: %d != %d", version, MdbDataVersion)
		return
	}
	pos += 4
	pos += 8 // Fixed address
	pos += 8 // Map size
	pos, freeRoot, freeDepth = readDbRecord(page[:], pos)
	pos, mainRoot, mainDepth = readDbRecord(page[:], pos)
	pos += 8 // Last page
	txnID = binary.LittleEndian.Uint64(page[pos:])
	return
}

func readDbRecord(page []byte, pos int) (newpos int, rootPageID uint64, depth uint16) {
	pos += 4 // Padding (key size for fixed key databases)
	pos += 2 // Flags
	depth = binary.LittleEndian.Uint16(page[pos:])
	pos += 2 // Depth
	pos += 8 // Number of branch pages
	pos += 8 // Number of leaf pages
	pos += 8 // Number of overflow pages
	pos += 8 // Number of entries
	rootPageID = binary.LittleEndian.Uint64(page[pos:])
	pos += 8
	newpos = pos
	return
}

func readOverflowPageHeader(page []byte, pos int) (newpos int, flags uint16, overflowNum int) {
	pos += 8 // Page ID
	pos += 2 // Padding
	flags = binary.LittleEndian.Uint16(page[pos:])
	pos += 2
	overflowNum = int(binary.LittleEndian.Uint32(page[pos:]))
	pos += 4 // Overflow page number / lower upper bound of free space
	newpos = pos
	return
}
