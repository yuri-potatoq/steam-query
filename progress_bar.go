package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

type windowTable struct {
	sync.Mutex
	initialCursorPos int
	currentCursorPos int
	maxWindowWidth   int
	endOfTablePos    int
	lines            []*windowLine
	oldTermState     *term.State
}

type LineBlock interface {
	// Retrieve the current content of the block likelly updated. Should be used trougth a mutex.
	Content() string
	// Total percentage space filled into the line of horizontal size of the terminal.
	Percentage() float32
	// Init block buffer with the amount of chars available for the space allocated belong the line.
	Init(size int)
}

type lineBlockInfo struct {
	maxSize   float32
	lineBlock LineBlock
}

// The arguments with lines blocks must be sorted in the order you wish they visable on the terminal.
func (w *windowTable) addLine(blks ...LineBlock) (*windowLine, error) {
	w.Lock()
	defer w.Unlock()

	fmt.Println("")
	line := &windowLine{
		blocks: make([]*lineBlockInfo, len(blks)),
	}

	var remainingSpace float32 = 100 // 100% of the line
	for i, blk := range blks {
		percentage := blk.Percentage()
		blockInfo := lineBlockInfo{
			lineBlock: blk,
			maxSize:   (blk.Percentage() / 100) * float32(w.maxWindowWidth),
		}

		blk.Init(int(blockInfo.maxSize))

		if len(blk.Content()) > w.maxWindowWidth || percentage > remainingSpace {
			return nil, errors.New("too long block")
		}

		remainingSpace -= percentage
		line.blocks[i] = &blockInfo
	}

	w.lines = append(w.lines, line)

	return line, nil
}

type windowLine struct {
	blocks []*lineBlockInfo
}

func (w *windowTable) updateLines() {
	w.Lock()
	defer w.Unlock()
	if len(w.lines) < 1 {
		return
	}

	linesStr := ""
	for _, line := range w.lines {
		content := ""
		for _, blk := range line.blocks {
			content = fmt.Sprintf("%s%s", content, blk.lineBlock.Content())
		}
		linesStr = fmt.Sprintf("%s%s\033[1B\r", linesStr, content)
	}
	fmt.Printf("\033[%dF%s", len(w.lines), linesStr)
}

func (wt *windowTable) Close() {
	term.Restore(1, wt.oldTermState)
}

func SetupWindowTable() (*windowTable, error) {
	winMaxWidth, _, err := term.GetSize(1)
	if err != nil {
		return nil, err
	}

	state, err := term.MakeRaw(1)
	if err != nil {
		return nil, err
	}

	posRow, _, err := getCursorPos()
	if err != nil {
		return nil, err
	}

	return &windowTable{
		currentCursorPos: posRow,
		initialCursorPos: posRow,
		maxWindowWidth:   winMaxWidth,
		endOfTablePos:    posRow,
		oldTermState:     state,
	}, nil
}

func (wt *windowTable) RefreshRoutine(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				wt.updateLines()
				return
			default:
				wt.updateLines()
				time.Sleep(time.Millisecond * 30)
			}
		}
	}()
}

/**
 * Progress Bar Block
 */

type progressBarBlock struct {
	mu              sync.Mutex
	content         []rune
	widthPercentage float32
	completed       int
	progressSymbol  rune
}

func NewProgressBarBlock(widthPercentage float32, progressSymbol rune) *progressBarBlock {
	return &progressBarBlock{
		widthPercentage: widthPercentage,
		progressSymbol:  progressSymbol,
	}
}

func (pb *progressBarBlock) Content() string {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	return string(pb.content)
}

func (pb *progressBarBlock) Init(size int) {
	pb.content = make([]rune, size)
	pb.content[0] = '['
	pb.content[size-1] = ']'

	// fill all content block with empty char, skipping the end and start
	for i := 1; i < size-2; i++ {
		pb.content[i] += ' '
	}
}

func (pb *progressBarBlock) Percentage() float32 {
	return pb.widthPercentage
}

func (pb *progressBarBlock) Progress(percentage int) bool {
	pb.mu.Lock()
	defer pb.mu.Unlock()

	if pb.completed >= 100 {
		return true
	}

	sz := len(pb.content)
	pb.content[0] = '['
	pb.content[sz-1] = ']'

	// only calculate based on content size inside the brackets
	lastWrittenOffset := getTruncatedIndexFromPercentage(pb.completed, sz-2)
	nextOffset := getTruncatedIndexFromPercentage(pb.completed+percentage, sz-2)

	// just to avoid fill the first position if the progress char, start from index 1
	if lastWrittenOffset == 0 {
		lastWrittenOffset++
	}

	// skip last written position and only override remaining positions
	// only fill positions regarding the amount positions calculated by the given percentage
	for i := lastWrittenOffset; i < sz-1 && i < nextOffset; i++ {
		pb.content[i] = pb.progressSymbol
	}
	pb.completed += percentage
	return false
}

func getTruncatedIndexFromPercentage(percentage int, size int) int {
	return int(math.Floor((float64(percentage) / 100.0) * float64(size)))
}

/**
 * Blank block to fill blank space on the window
 */

type blankBlock struct {
	content         string
	widthPercentage float32
}

func (pb *blankBlock) Content() string {
	return pb.content
}

func (pb *blankBlock) Init(size int) {
	for range size {
		pb.content += " "
	}
}

func (pb *blankBlock) Percentage() float32 {
	return pb.widthPercentage
}

/**
 * Info block. Fills given block space percentage with information.
 */

type infoBlock struct {
	mu              sync.Mutex
	maxContentSize  int
	content         string
	widthPercentage float32
}

func (ib *infoBlock) Init(size int) {
	for range size {
		ib.content += " "
	}
	ib.maxContentSize = size
}

func (ib *infoBlock) Percentage() float32 {
	return ib.widthPercentage
}

func (ib *infoBlock) Content() string {
	return ib.content
}

func (ib *infoBlock) Update(text string) {
	ib.mu.Lock()
	defer ib.mu.Unlock()

	if sz := len(text); sz < ib.maxContentSize {
		ib.content = text + strings.Repeat(" ", ib.maxContentSize-sz)
	} else {
		ib.content = text[:ib.maxContentSize]
	}
}

type ProgressLine struct {
	progress     *progressBarBlock
	blankPadding *blankBlock
	info         *infoBlock
}

func NewProgressLine() *ProgressLine {
	return &ProgressLine{
		info:         &infoBlock{widthPercentage: 40},
		blankPadding: &blankBlock{widthPercentage: 20},
		progress:     NewProgressBarBlock(40, '='),
	}
}

func (pline *ProgressLine) Progress(percentage int) bool {
	return pline.progress.Progress(percentage)
}

func (pline *ProgressLine) UpdateInfo(info string) {
	pline.info.Update(info)
}

func (pline *ProgressLine) Blocks() []LineBlock {
	return []LineBlock{
		pline.info,
		pline.blankPadding,
		pline.progress,
	}
}
