package weblog

import "time"

// LineAt is a parsed access-log line paired with End: the absolute byte offset
// in the file immediately after this line (including its newline). The tailer
// uses End to advance its persisted cursor exactly to a finalized line.
type LineAt struct {
	Entry AccessEntry
	End   int64
}

// PartitionClosed decides which of the freshly-read lines belong to buckets
// that have already closed (bucketSize window ending at or before now's current
// bucket) and are therefore safe to finalize, and how far the cursor may
// advance.
//
// It returns the longest leading run of lines that are ALL in closed buckets.
// It stops at the first line whose bucket is still open, so a still-filling
// bucket is never finalized early and the cursor never skips an un-finalized
// line — even if the log is written slightly out of order. Lines after the cut
// are held back (re-read on the next run). advanceTo is the End offset of the
// last finalized line; hasClosed is false when nothing is ready yet (caller
// must then keep its old cursor and persist nothing).
//
// Pure and deterministic: no I/O, no clock read (now is passed in).
func PartitionClosed(lines []LineAt, now time.Time, bucketSize time.Duration) (closed []AccessEntry, advanceTo int64, hasClosed bool) {
	if bucketSize <= 0 {
		bucketSize = time.Hour
	}
	openBucketStart := now.Truncate(bucketSize)
	for _, l := range lines {
		// A line is in a closed bucket iff its bucket start is strictly before
		// the currently-open bucket start.
		if !l.Entry.Time.Truncate(bucketSize).Before(openBucketStart) {
			break // first open-bucket line: hold it and everything after.
		}
		closed = append(closed, l.Entry)
		advanceTo = l.End
		hasClosed = true
	}
	return closed, advanceTo, hasClosed
}
