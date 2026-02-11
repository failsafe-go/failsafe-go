package util

// QuantileWindow maintains an exact quantile over a sliding window of samples using a dual-linked list structure.
// It provides O(1) quantile queries and O(k) insertion where k is the distance from the current quantile
// (typically very small in stable systems).
//
// This type is not concurrency safe.
type QuantileWindow struct {
	quantile float64
	maxSize  int
	size     int

	// Time-ordered doubly-linked list (for sliding window)
	timeHead *quantileNode
	timeTail *quantileNode

	// Value-ordered doubly-linked list (for quantile lookup)
	valueHead *quantileNode

	// Anchor pointer to target quantile node
	quantileNode *quantileNode
}

type quantileNode struct {
	value float64

	// Time ordering (insertion order)
	timeNext *quantileNode
	timePrev *quantileNode

	// Value ordering (sorted by value)
	valueNext *quantileNode
	valuePrev *quantileNode
}

// NewQuantileWindow creates a new QuantileWindow for the given quantile (0-1) and maximum window size.
// For example, quantile=0.9 tracks the p90 value.
func NewQuantileWindow(quantile float64, maxSize int) *QuantileWindow {
	return &QuantileWindow{
		quantile: quantile,
		maxSize:  maxSize,
	}
}

// Add adds a sample to the window and returns the updated quantile value.
// If the window is full, the oldest sample is automatically removed.
func (w *QuantileWindow) Add(value float64) float64 {
	node := &quantileNode{value: value}

	// Insert at tail of time-ordered list (newest)
	w.insertTimeOrdered(node)

	// Insert into value-ordered list (starting from quantile anchor for efficiency)
	w.insertValueOrdered(node)

	// Remove oldest if we exceeded capacity
	if w.size > w.maxSize {
		w.removeOldest()
	}

	// Update quantile anchor
	w.updateQuantileAnchor()

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
}

// removeOldest removes the head of the time-ordered list (oldest sample)
func (w *QuantileWindow) removeOldest() {
	if w.timeHead == nil {
		return
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

	// Mark quantile anchor as stale if we removed it
	if w.quantileNode == oldNode {
		w.quantileNode = nil
	}

	w.size--
}

// updateQuantileAnchor updates the quantile anchor to point to the node at the target quantile position.
// This uses the existing anchor as a starting point for efficient O(k) updates in stable systems.
func (w *QuantileWindow) updateQuantileAnchor() {
	if w.size == 0 {
		w.quantileNode = nil
		return
	}

	// Calculate target position (0-indexed)
	targetPos := int(float64(w.size-1) * w.quantile)

	// If no anchor yet, traverse from head
	if w.quantileNode == nil {
		w.quantileNode = w.valueHead
		for i := 0; i < targetPos && w.quantileNode != nil; i++ {
			w.quantileNode = w.quantileNode.valueNext
		}
		return
	}

	// Determine current position of anchor by counting from head
	// In a more optimized version, we'd track this incrementally
	currentPos := 0
	curr := w.valueHead
	for curr != nil && curr != w.quantileNode {
		currentPos++
		curr = curr.valueNext
	}

	// Move anchor forward or backward as needed
	if currentPos < targetPos {
		// Move forward
		for currentPos < targetPos && w.quantileNode.valueNext != nil {
			w.quantileNode = w.quantileNode.valueNext
			currentPos++
		}
	} else if currentPos > targetPos {
		// Move backward
		for currentPos > targetPos && w.quantileNode.valuePrev != nil {
			w.quantileNode = w.quantileNode.valuePrev
			currentPos--
		}
	}
}
