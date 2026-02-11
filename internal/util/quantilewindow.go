package util

import "time"

// QuantileWindow maintains an exact quantile over a time-based sliding window of samples using a dual-linked list structure.
// It provides O(1) quantile queries and O(k) insertion where k is the distance from the current quantile
// (typically very small in stable systems).
//
// This type is not concurrency safe.
type QuantileWindow struct {
	quantile    float64
	maxDuration time.Duration
	size        int

	// Time-ordered doubly-linked list (for sliding window)
	timeHead *quantileNode
	timeTail *quantileNode

	// Value-ordered doubly-linked list (for quantile lookup)
	valueHead *quantileNode

	// Anchor pointer to target quantile node
	quantileNode     *quantileNode
	quantilePosition int // Current 0-indexed position of quantileNode in sorted order
}

type quantileNode struct {
	value     float64 // 8 bytes
	timestamp int64   // 8 bytes (Unix nanoseconds - saves 16 bytes vs time.Time!)

	// Time ordering (insertion order)
	timeNext *quantileNode // 8 bytes
	timePrev *quantileNode // 8 bytes

	// Value ordering (sorted by value)
	valueNext *quantileNode // 8 bytes
	valuePrev *quantileNode // 8 bytes
}

// Total: 48 bytes per node (vs 64 bytes with time.Time = 25% reduction!)

// NewQuantileWindow creates a new QuantileWindow for the given quantile (0-1) and maximum time duration.
// For example, quantile=0.9 with maxDuration=1*time.Minute tracks the p90 value over the last minute.
// Samples older than maxDuration are automatically expired.
func NewQuantileWindow(quantile float64, maxDuration time.Duration) *QuantileWindow {
	return &QuantileWindow{
		quantile:    quantile,
		maxDuration: maxDuration,
	}
}

// Add adds a sample to the window and returns the updated quantile value.
// Samples older than maxDuration are automatically expired before adding the new sample.
func (w *QuantileWindow) Add(value float64) float64 {
	return w.AddWithTime(value, time.Now())
}

// AddWithTime adds a sample with an explicit timestamp. This is useful for testing or when
// samples are collected at a specific time. Samples older than maxDuration from the given
// timestamp are automatically expired.
func (w *QuantileWindow) AddWithTime(value float64, timestamp time.Time) float64 {
	timestampNanos := timestamp.UnixNano()

	// Remove expired entries (older than maxDuration)
	// Reuse the first removed node if available (zero-allocation steady state!)
	cutoffNanos := timestamp.Add(-w.maxDuration).UnixNano()
	var nodeToReuse *quantileNode
	if w.timeHead != nil && w.timeHead.timestamp < cutoffNanos {
		nodeToReuse = w.removeOldest()
	}
	// Continue removing any additional expired entries
	for w.timeHead != nil && w.timeHead.timestamp < cutoffNanos {
		w.removeOldest()
	}

	// Reuse node if we removed one, otherwise allocate new
	var node *quantileNode
	if nodeToReuse != nil {
		node = nodeToReuse
	} else {
		node = &quantileNode{}
	}

	// Set values
	node.value = value
	node.timestamp = timestampNanos

	// Insert at tail of time-ordered list (newest)
	w.insertTimeOrdered(node)

	// Insert into value-ordered list (starting from quantile anchor for efficiency)
	// This also updates quantilePosition if needed
	w.insertValueOrdered(node)

	// Update quantile anchor to target position
	w.updateQuantilePosition()

	return w.Value()
}

// Value returns the current quantile value, or 0 if the window is empty.
func (w *QuantileWindow) Value() float64 {
	if w.quantileNode == nil {
		return 0
	}
	return w.quantileNode.value
}

// Size returns the current number of samples in the window.
func (w *QuantileWindow) Size() int {
	return w.size
}

// Reset clears all samples from the window.
func (w *QuantileWindow) Reset() {
	w.timeHead = nil
	w.timeTail = nil
	w.valueHead = nil
	w.quantileNode = nil
	w.quantilePosition = 0
	w.size = 0
}

// insertTimeOrdered appends a node to the end of the time-ordered list (O(1))
func (w *QuantileWindow) insertTimeOrdered(node *quantileNode) {
	if w.timeHead == nil {
		w.timeHead = node
		w.timeTail = node
	} else {
		w.timeTail.timeNext = node
		node.timePrev = w.timeTail
		w.timeTail = node
	}
	w.size++
}

// insertValueOrdered inserts a node into the value-ordered list, starting search from the quantile anchor.
// This gives O(k) performance where k is the distance from the quantile (typically small).
// Also updates quantilePosition if the new node is inserted before the quantile.
func (w *QuantileWindow) insertValueOrdered(node *quantileNode) {
	// Empty list
	if w.valueHead == nil {
		w.valueHead = node
		return
	}

	// Start search from quantile anchor if available, otherwise from head
	var start *quantileNode
	if w.quantileNode != nil {
		start = w.quantileNode
	} else {
		start = w.valueHead
	}

	// Determine search direction
	if node.value < start.value {
		// Search backward toward smaller values
		curr := start
		for curr.valuePrev != nil && curr.valuePrev.value > node.value {
			curr = curr.valuePrev
		}

		// Insert before curr
		node.valueNext = curr
		node.valuePrev = curr.valuePrev
		if curr.valuePrev != nil {
			curr.valuePrev.valueNext = node
		} else {
			w.valueHead = node
		}
		curr.valuePrev = node
	} else {
		// Search forward toward larger values
		curr := start
		for curr.valueNext != nil && curr.valueNext.value < node.value {
			curr = curr.valueNext
		}

		// Insert after curr
		node.valuePrev = curr
		node.valueNext = curr.valueNext
		if curr.valueNext != nil {
			curr.valueNext.valuePrev = node
		}
		curr.valueNext = node
	}

	// If we have a quantile node and the new node is before it, increment position
	if w.quantileNode != nil && node.value < w.quantileNode.value {
		w.quantilePosition++
	}
}

// removeOldest removes the oldest node and returns it for reuse. It also updates quantilePosition to an approximate
// position, but updateQuantilePosition must be called to recompute an accurate quantilePosition.
// Returns nil if there are no nodes to remove.
func (w *QuantileWindow) removeOldest() *quantileNode {
	if w.timeHead == nil {
		return nil
	}

	oldNode := w.timeHead

	// Remove from time-ordered list (O(1))
	w.timeHead = oldNode.timeNext
	if w.timeHead != nil {
		w.timeHead.timePrev = nil
	} else {
		w.timeTail = nil
	}

	// Remove from value-ordered list (O(1))
	if oldNode.valuePrev != nil {
		oldNode.valuePrev.valueNext = oldNode.valueNext
	} else {
		w.valueHead = oldNode.valueNext
	}
	if oldNode.valueNext != nil {
		oldNode.valueNext.valuePrev = oldNode.valuePrev
	}

	// Update quantile tracking
	if w.quantileNode == oldNode {
		// Case 1: Removed the quantile node itself
		if oldNode.valueNext != nil {
			// Move to next node in sorted order - it's now at our old position
			w.quantileNode = oldNode.valueNext
			// quantilePosition stays the same - valueNext is now at this index
		} else if oldNode.valuePrev != nil {
			// No next node, move to previous - we're now one position back
			w.quantileNode = oldNode.valuePrev
			w.quantilePosition--
		} else {
			// This was the only node - window is now empty
			w.quantileNode = nil
			w.quantilePosition = 0
		}
	} else if w.quantileNode != nil && oldNode.value < w.quantileNode.value {
		// Case 2: Removed a node before the quantile in sorted order
		// All nodes shift down by one position, so decrement quantile position
		w.quantilePosition--
	}

	w.size--

	// Clear pointers before returning for reuse
	oldNode.timeNext = nil
	oldNode.timePrev = nil
	oldNode.valueNext = nil
	oldNode.valuePrev = nil
	return oldNode
}

// updateQuantilePosition updates the quantile anchor to point to the node at the target quantile position.
// Uses the tracked quantilePosition for O(k) updates where k is typically very small.
func (w *QuantileWindow) updateQuantilePosition() {
	if w.size == 0 {
		w.quantileNode = nil
		w.quantilePosition = 0
		return
	}

	// Calculate target position (0-indexed)
	targetPos := int(float64(w.size-1) * w.quantile)

	// If no anchor yet, traverse from head
	if w.quantileNode == nil {
		w.quantileNode = w.valueHead
		w.quantilePosition = 0
		for w.quantilePosition < targetPos && w.quantileNode != nil && w.quantileNode.valueNext != nil {
			w.quantileNode = w.quantileNode.valueNext
			w.quantilePosition++
		}
		return
	}

	// Move anchor forward or backward as needed using tracked position
	if w.quantilePosition < targetPos {
		// Move forward
		for w.quantilePosition < targetPos && w.quantileNode.valueNext != nil {
			w.quantileNode = w.quantileNode.valueNext
			w.quantilePosition++
		}
	} else if w.quantilePosition > targetPos {
		// Move backward
		for w.quantilePosition > targetPos && w.quantileNode.valuePrev != nil {
			w.quantileNode = w.quantileNode.valuePrev
			w.quantilePosition--
		}
	}
}
