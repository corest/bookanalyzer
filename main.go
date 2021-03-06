package main

import (
	"bufio"
	"flag"
	"os"

	"github.com/corest/bookanalyzer/pkg/orderbook"
)

func main() {
	targetSize := flag.Int("target-size", 0, "Target size for trading")
	flag.Parse()

	if *targetSize <= 0 {
		panic("-target-size must be > 0")
	}

	orderBook := orderbook.New(*targetSize)

	scanner := bufio.NewScanner(os.Stdin)

	err := orderBook.Process(scanner)
	if err != nil {
		panic(err)
	}
}
