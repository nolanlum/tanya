package gateway

import (
	"errors"
	"log"
	"strconv"
	"strings"
)

func (sc *SlackClient) shouldQuoteThreadParent(threadTs, messageTs string) bool {
	latestThreadTs, found := sc.threadTimestamps.Get(threadTs)
	sc.threadTimestamps.Add(threadTs, messageTs)

	if !found {
		return sc.threadQuoteInterval != 0
	}

	// Try to parse the slack ts as an integer... not sure if this will always work?
	latestThreadTsInt, err := parseTsToInt(latestThreadTs.(string))
	if err != nil {
		log.Printf("Could not parse thread ts %v, not quoting original message: %v", latestThreadTs, err)
		return false
	}
	messageTsInt, err := parseTsToInt(messageTs)
	if err != nil {
		log.Printf("Could not parse message ts %v, not quoting original message: %v", latestThreadTs, err)
		return false
	}

	return messageTsInt-latestThreadTsInt > sc.threadQuoteInterval
}

func parseTsToInt(ts string) (int, error) {
	for _, c := range ts {
		if !(c >= '0' && c <= '9' || c == '.') {
			return 0, errors.New("ts is not a number")
		}
	}

	sep := strings.LastIndex(ts, ".")
	if sep < 0 {
		sep = len(ts)
	}

	i, err := strconv.ParseInt(ts[0:sep], 10, 32)
	if err != nil {
		return 0, err
	}

	return int(i), nil
}
