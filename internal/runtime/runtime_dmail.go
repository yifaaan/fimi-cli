package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"fimi-cli/internal/contextstore"
)

func (r Runner) persistRunStart(
	ctx context.Context,
	store contextstore.Context,
	prompt string,
	userRecord contextstore.TextRecord,
) error {
	return r.shieldContextWrite(ctx, func() error {
		checkpointID, err := store.AppendCheckpointWithMetadata(contextstore.CheckpointMetadata{
			CreatedAt:     time.Now().UTC().Format(time.RFC3339),
			PromptPreview: checkpointPromptPreview(prompt),
		})
		if err != nil {
			return err
		}
		if r.dmailer != nil {
			r.dmailer.SetCheckpointCount(checkpointID + 1)
			if err := r.appendCheckpointMarker(store, checkpointID); err != nil {
				return err
			}
		}
		return store.Append(userRecord)
	})
}

func (r Runner) persistStepCheckpoint(ctx context.Context, store contextstore.Context) error {
	if r.dmailer == nil {
		return nil
	}

	return r.shieldContextWrite(ctx, func() error {
		checkpointID, err := store.AppendCheckpointWithMetadata(contextstore.CheckpointMetadata{
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			return err
		}
		r.dmailer.SetCheckpointCount(checkpointID + 1)
		return r.appendCheckpointMarker(store, checkpointID)
	})
}

func (r Runner) applyPendingDMail(ctx context.Context, store contextstore.Context) (bool, error) {
	if r.dmailer == nil {
		return false, nil
	}

	message, checkpointID, ok := r.dmailer.Fetch()
	if !ok {
		return false, nil
	}

	err := r.shieldContextWrite(ctx, func() error {
		if _, err := store.RevertToCheckpoint(checkpointID); err != nil {
			return err
		}
		newCheckpointID, err := store.AppendCheckpointWithMetadata(contextstore.CheckpointMetadata{
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			return err
		}
		r.dmailer.SetCheckpointCount(newCheckpointID + 1)
		if err := r.appendCheckpointMarker(store, newCheckpointID); err != nil {
			return err
		}
		dmailContent := fmt.Sprintf("<system>D-Mail received: %s</system>\n\nRead the D-Mail above carefully. Act on the information it contains. Do NOT mention the D-Mail mechanism or time travel to the user.", message)
		return store.Append(contextstore.NewUserTextRecord(dmailContent))
	})
	if err != nil {
		return true, err
	}

	return true, nil
}

func (r Runner) appendCheckpointMarker(store contextstore.Context, checkpointID int) error {
	return store.Append(contextstore.NewUserTextRecord(
		fmt.Sprintf("<system>CHECKPOINT %d</system>", checkpointID),
	))
}

func checkpointPromptPreview(prompt string) string {
	preview := strings.Join(strings.Fields(strings.TrimSpace(prompt)), " ")
	if len(preview) <= checkpointPromptPreviewMaxLen {
		return preview
	}

	return preview[:checkpointPromptPreviewMaxLen-3] + "..."
}
