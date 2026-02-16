package cmd

import (
	"errors"
	"testing"

	"github.com/dotcommander/yai/internal/config"
	"github.com/stretchr/testify/require"
)

var flagParseErrorTests = []struct {
	in     string
	flag   string
	reason string
}{
	{
		"unknown flag: --nope",
		"--nope",
		"Flag %s is missing.",
	},
	{
		"flag needs an argument: --delete",
		"--delete",
		"Flag %s needs an argument.",
	},
	{
		"flag needs an argument: 'd' in -d",
		"-d",
		"Flag %s needs an argument.",
	},
	{
		`invalid argument "20dd" for "--delete-older-than" flag: time: unknown unit "dd" in duration "20dd"`,
		"--delete-older-than",
		"Flag %s have an invalid argument.",
	},
	{
		`invalid argument "sdfjasdl" for "--max-tokens" flag: strconv.ParseInt: parsing "sdfjasdl": invalid syntax`,
		"--max-tokens",
		"Flag %s have an invalid argument.",
	},
	{
		`invalid argument "nope" for "-r, --raw" flag: strconv.ParseBool: parsing "nope": invalid syntax`,
		"-r, --raw",
		"Flag %s have an invalid argument.",
	},
}

func TestFlagParseError(t *testing.T) {
	for _, tf := range flagParseErrorTests {
		t.Run(tf.in, func(t *testing.T) {
			err := newFlagParseError(errors.New(tf.in))
			require.Equal(t, tf.flag, err.Flag())
			require.Equal(t, tf.reason, err.ReasonFormat())
			require.Equal(t, tf.in, err.Error())
		})
	}
}

func TestMaxCompletionTokensFlag(t *testing.T) {
	t.Run("flag is registered and can be parsed", func(t *testing.T) {
		cfg := config.Config{}
		cmd := NewRootCmd(BuildInfo{}, cfg, nil)

		err := cmd.ParseFlags([]string{"--max-completion-tokens", "4096"})
		require.NoError(t, err)

		flag := cmd.Flag("max-completion-tokens")
		require.NotNil(t, flag)
		require.Equal(t, "4096", flag.Value.String())
	})

	t.Run("accepts zero value", func(t *testing.T) {
		cfg := config.Config{}
		cmd := NewRootCmd(BuildInfo{}, cfg, nil)

		err := cmd.ParseFlags([]string{"--max-completion-tokens", "0"})
		require.NoError(t, err)

		flag := cmd.Flag("max-completion-tokens")
		require.NotNil(t, flag)
		require.Equal(t, "0", flag.Value.String())
	})
}
