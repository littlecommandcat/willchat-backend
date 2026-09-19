package core

import (
	"fmt"
	"strconv"
	"sync"
	"time"
)

const (
	snowflakeEpoch int64 = 1577836800000
	workerBits           = 10
	sequenceBits         = 12
	maxWorkerID    int64 = (1 << workerBits) - 1
	maxSequence    int64 = (1 << sequenceBits) - 1
)

// Snowflake
type Snowflake struct {
	mu            sync.Mutex
	workerID      int64
	lastTimestamp int64
	sequence      int64
}

func NewSnowflake(workerID int64) *Snowflake {
	if workerID < 0 || workerID > maxWorkerID {
		workerID = 1
	}

	return &Snowflake{
		workerID: workerID,
	}
}

func (s *Snowflake) Next() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	timestamp := time.Now().UnixMilli() - snowflakeEpoch

	if timestamp < s.lastTimestamp {
		timestamp = s.lastTimestamp
	}

	if timestamp == s.lastTimestamp {
		s.sequence = (s.sequence + 1) & maxSequence

		if s.sequence == 0 {
			for timestamp <= s.lastTimestamp {
				timestamp = time.Now().UnixMilli() - snowflakeEpoch
			}
		}
	} else {
		s.sequence = 0
	}

	s.lastTimestamp = timestamp

	id := (timestamp << (workerBits + sequenceBits)) |
		(s.workerID << sequenceBits) |
		s.sequence

	return strconv.FormatInt(id, 10)
}

// ToTime
func (s *Snowflake) ToTime(snowflakeStr string) (int64, error) {
	snowflakeID, err := strconv.ParseInt(snowflakeStr, 10, 64)
	if err != nil {
		return 0, err
	}

	relativeMillis := snowflakeID >> (workerBits + sequenceBits)
	absoluteMillis := relativeMillis + snowflakeEpoch

	if absoluteMillis < snowflakeEpoch {
		return 0, fmt.Errorf("invalid snowflake ID: timestamp before epoch")
	}

	return absoluteMillis, nil
}
