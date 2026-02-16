package cmd

import (
	"errors"
	"fmt"

	"github.com/charmbracelet/x/exp/ordered"
	"github.com/dotcommander/yai/internal/config"
	"github.com/dotcommander/yai/internal/errs"
	"github.com/dotcommander/yai/internal/storage"
)

type conversationPlan struct {
	WriteID string
	Title   string
	ReadID  string
	API     string
	Model   string
}

func planConversation(cfg *config.Config, db *storage.DB) (conversationPlan, error) {
	continueLast := cfg.ContinueLast || (cfg.Continue != "" && cfg.Title == "")
	readID := ordered.First(cfg.Continue, cfg.Show)
	writeID := ordered.First(cfg.Title, cfg.Continue)
	title := writeID
	model := cfg.Model
	api := cfg.API

	if readID != "" || continueLast || cfg.ShowLast {
		found, err := findReadConversation(cfg, db, readID)
		if err != nil {
			return conversationPlan{}, errs.Wrap(err, "Could not find the conversation.")
		}
		if found != nil {
			readID = found.ID
			if found.Model != nil && found.API != nil {
				model = *found.Model
				api = *found.API
			}
		}
	}

	// if we are continuing last, update the existing conversation
	if continueLast {
		writeID = readID
	}

	if writeID == "" {
		writeID = storage.NewConversationID()
	}

	if !storage.SHA1Regexp.MatchString(writeID) {
		convo, err := db.Find(writeID)
		if err != nil {
			// it's a new conversation with a title
			writeID = storage.NewConversationID()
		} else {
			writeID = convo.ID
		}
	}

	return conversationPlan{
		WriteID: writeID,
		Title:   title,
		ReadID:  readID,
		API:     api,
		Model:   model,
	}, nil
}

func findReadConversation(cfg *config.Config, db *storage.DB, in string) (*storage.Conversation, error) {
	convo, err := db.Find(in)
	if err == nil {
		return convo, nil
	}
	if errors.Is(err, storage.ErrNoMatches) && cfg.Show == "" {
		convo, err := db.FindHEAD()
		if err != nil {
			return nil, fmt.Errorf("find latest conversation: %w", err)
		}
		return convo, nil
	}
	return nil, fmt.Errorf("find conversation: %w", err)
}
