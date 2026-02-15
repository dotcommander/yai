package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func testDB(tb testing.TB) *DB {
	db, err := Open(":memory:")
	require.NoError(tb, err)
	tb.Cleanup(func() {
		require.NoError(tb, db.Close())
	})
	return db
}

func TestDB(t *testing.T) {
	const testid = "df31ae23ab8b75b5643c2f846c570997edc71333"

	t.Run("list-empty", func(t *testing.T) {
		db := testDB(t)
		list := db.List()
		require.Empty(t, list)
	})

	t.Run("save", func(t *testing.T) {
		db := testDB(t)

		require.NoError(t, db.Save(testid, "message 1", "openai", "gpt-4o"))

		convo, err := db.Find("df31")
		require.NoError(t, err)
		require.Equal(t, testid, convo.ID)
		require.Equal(t, "message 1", convo.Title)

		list := db.List()
		require.Len(t, list, 1)
	})

	t.Run("save no id", func(t *testing.T) {
		db := testDB(t)
		require.Error(t, db.Save("", "message 1", "openai", "gpt-4o"))
	})

	t.Run("save no message", func(t *testing.T) {
		db := testDB(t)
		require.Error(t, db.Save(NewConversationID(), "", "openai", "gpt-4o"))
	})

	t.Run("update", func(t *testing.T) {
		db := testDB(t)

		require.NoError(t, db.Save(testid, "message 1", "openai", "gpt-4o"))
		time.Sleep(100 * time.Millisecond)
		require.NoError(t, db.Save(testid, "message 2", "openai", "gpt-4o"))

		convo, err := db.Find("df31")
		require.NoError(t, err)
		require.Equal(t, testid, convo.ID)
		require.Equal(t, "message 2", convo.Title)

		list := db.List()
		require.Len(t, list, 1)
	})

	t.Run("find head single", func(t *testing.T) {
		db := testDB(t)

		require.NoError(t, db.Save(testid, "message 2", "openai", "gpt-4o"))

		head, err := db.FindHEAD()
		require.NoError(t, err)
		require.Equal(t, testid, head.ID)
		require.Equal(t, "message 2", head.Title)
	})

	t.Run("find head multiple", func(t *testing.T) {
		db := testDB(t)

		require.NoError(t, db.Save(testid, "message 2", "openai", "gpt-4o"))
		time.Sleep(time.Millisecond * 100)
		nextConvo := NewConversationID()
		require.NoError(t, db.Save(nextConvo, "another message", "openai", "gpt-4o"))

		head, err := db.FindHEAD()
		require.NoError(t, err)
		require.Equal(t, nextConvo, head.ID)
		require.Equal(t, "another message", head.Title)

		list := db.List()
		require.Len(t, list, 2)
	})

	t.Run("find by title", func(t *testing.T) {
		db := testDB(t)

		require.NoError(t, db.Save(NewConversationID(), "message 1", "openai", "gpt-4o"))
		require.NoError(t, db.Save(testid, "message 2", "openai", "gpt-4o"))

		convo, err := db.Find("message 2")
		require.NoError(t, err)
		require.Equal(t, testid, convo.ID)
		require.Equal(t, "message 2", convo.Title)
	})

	t.Run("find match nothing", func(t *testing.T) {
		db := testDB(t)
		require.NoError(t, db.Save(testid, "message 1", "openai", "gpt-4o"))
		_, err := db.Find("message")
		require.ErrorIs(t, err, ErrNoMatches)
	})

	t.Run("find match many", func(t *testing.T) {
		db := testDB(t)
		const testid2 = "df31ae23ab9b75b5641c2f846c571000edc71315"
		require.NoError(t, db.Save(testid, "message 1", "openai", "gpt-4o"))
		require.NoError(t, db.Save(testid2, "message 2", "openai", "gpt-4o"))
		_, err := db.Find("df31ae")
		require.ErrorIs(t, err, ErrManyMatches)
	})

	t.Run("delete", func(t *testing.T) {
		db := testDB(t)

		require.NoError(t, db.Save(testid, "message 1", "openai", "gpt-4o"))
		require.NoError(t, db.Delete(NewConversationID()))

		list := db.List()
		require.NotEmpty(t, list)

		for _, item := range list {
			require.NoError(t, db.Delete(item.ID))
		}

		list = db.List()
		require.Empty(t, list)
	})

	t.Run("completions", func(t *testing.T) {
		db := testDB(t)

		const testid1 = "fc5012d8c67073ea0a46a3c05488a0e1d87df74b"
		const title1 = "some title"
		const testid2 = "6c33f71694bf41a18c844a96d1f62f153e5f6f44"
		const title2 = "football teams"
		require.NoError(t, db.Save(testid1, title1, "openai", "gpt-4o"))
		require.NoError(t, db.Save(testid2, title2, "openai", "gpt-4o"))

		results := db.Completions("f")
		require.Equal(t, []string{
			fmt.Sprintf("%s\t%s", testid1[:SHA1Short], title1),
			fmt.Sprintf("%s\t%s", title2, testid2[:SHA1Short]),
		}, results)

		results = db.Completions(testid1[:8])
		require.Equal(t, []string{
			fmt.Sprintf("%s\t%s", testid1, title1),
		}, results)
	})

	t.Run("persists to jsonl index", func(t *testing.T) {
		dir := t.TempDir()

		db, err := Open(dir)
		require.NoError(t, err)
		require.NoError(t, db.Save(testid, "message 1", "openai", "gpt-4o"))
		require.NoError(t, db.Close())

		db2, err := Open(dir)
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, db2.Close())
		})

		convo, err := db2.Find(testid[:8])
		require.NoError(t, err)
		require.Equal(t, testid, convo.ID)

		_, err = os.Stat(filepath.Join(dir, indexFileName))
		require.NoError(t, err)
	})
}
