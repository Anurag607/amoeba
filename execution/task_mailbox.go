package execution

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// TaskMessage carries only a payload digest; hosts retain message content in
// their encrypted task store. Sequence is assigned by the mailbox.
type TaskMessage struct {
	ID              string    `json:"id"`
	Owner           Identity  `json:"owner"`
	SenderTaskID    string    `json:"sender_task_id"`
	RecipientTaskID string    `json:"recipient_task_id"`
	PayloadHash     string    `json:"payload_hash"`
	Sequence        uint64    `json:"sequence"`
	CreatedAt       time.Time `json:"created_at"`
}

type TaskMailboxState struct {
	HeadSequence  uint64 `json:"head_sequence"`
	AckedSequence uint64 `json:"acked_sequence"`
}

type TaskMailbox interface {
	Append(context.Context, TaskMessage) (TaskMessage, bool, error)
	Replay(context.Context, Identity, string, uint64, int) ([]TaskMessage, error)
	Ack(context.Context, Identity, string, uint64) (TaskMailboxState, error)
}

type MemoryTaskMailbox struct {
	mu       sync.Mutex
	messages map[string][]TaskMessage
	acked    map[string]uint64
}

func NewMemoryTaskMailbox() *MemoryTaskMailbox {
	return &MemoryTaskMailbox{messages: make(map[string][]TaskMessage), acked: make(map[string]uint64)}
}

func (m *MemoryTaskMailbox) Append(ctx context.Context, message TaskMessage) (TaskMessage, bool, error) {
	if err := ctx.Err(); err != nil {
		return TaskMessage{}, false, err
	}
	if message.ID == "" || message.SenderTaskID == "" || message.RecipientTaskID == "" || message.PayloadHash == "" || message.Owner.Validate() != nil {
		return TaskMessage{}, false, fmt.Errorf("task message requires ID, owner, sender, recipient, and payload hash")
	}
	key := mailboxKey(message.Owner, message.RecipientTaskID)
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.messages[key] {
		if existing.ID != message.ID {
			continue
		}
		if existing.SenderTaskID != message.SenderTaskID || existing.PayloadHash != message.PayloadHash {
			return TaskMessage{}, false, fmt.Errorf("task message ID reused with conflicting payload")
		}
		return existing, false, nil
	}
	message.Sequence = uint64(len(m.messages[key]) + 1)
	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now()
	}
	m.messages[key] = append(m.messages[key], message)
	return message, true, nil
}

func (m *MemoryTaskMailbox) Replay(ctx context.Context, owner Identity, recipient string, after uint64, limit int) ([]TaskMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("task mailbox replay limit must be positive")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]TaskMessage, 0, limit)
	for _, message := range m.messages[mailboxKey(owner, recipient)] {
		if message.Sequence > after {
			out = append(out, message)
			if len(out) == limit {
				break
			}
		}
	}
	return out, nil
}

func (m *MemoryTaskMailbox) Ack(ctx context.Context, owner Identity, recipient string, sequence uint64) (TaskMailboxState, error) {
	if err := ctx.Err(); err != nil {
		return TaskMailboxState{}, err
	}
	key := mailboxKey(owner, recipient)
	m.mu.Lock()
	defer m.mu.Unlock()
	head := uint64(len(m.messages[key]))
	if sequence < m.acked[key] || sequence > head {
		return TaskMailboxState{}, fmt.Errorf("invalid task mailbox acknowledgement")
	}
	m.acked[key] = sequence
	return TaskMailboxState{HeadSequence: head, AckedSequence: sequence}, nil
}

func mailboxKey(owner Identity, recipient string) string {
	return owner.PrincipalID + "\x00" + owner.TenantID + "\x00" + owner.SessionID + "\x00" + recipient
}
