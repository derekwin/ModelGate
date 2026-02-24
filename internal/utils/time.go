package utils

import "time"

func CurrentTimestamp() int64 {
    return time.Now().Unix()
}
