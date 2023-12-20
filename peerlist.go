package basecrawler

import (
	"encoding/csv"
	"io"
	"net"
	"os"
	"strings"
)

// MustParsePeerList is like [ParsePeerList] but will panic if it encounters an error.
func MustParsePeerList(path string) []string {
	peerlist, err := ParsePeerList(path)
	if err != nil {
		panic(err)
	}
	return peerlist
}

// ParsePeerList takes a CSV file of peers (in format ip:port), checks that it's valid and transforms it to a string slice.
//
// It can be used by crawler implementations to parse a peerlist needed to bootstrap a crawler in a common way.
func ParsePeerList(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	var peerlist []string

	r := csv.NewReader(file)

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		// Not necessarily needed (because the basecrawler would trim these as well) but for good measure, we already trim here
		peer := strings.Trim(record[0], "\"")

		host, port, err := net.SplitHostPort(peer)
		if err != nil {
			return nil, err
		}

		peerlist = append(peerlist, net.JoinHostPort(host, port))
	}

	return peerlist, nil
}
