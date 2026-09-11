package progress

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStepperLines(t *testing.T) {
	t.Run("Happy Path scenario", func(t *testing.T) {
		var buf bytes.Buffer
		p := New(&buf, 3)

		p.Step("Analysis Window")
		p.Done("12 months")
		p.Step("Fetch GitHub Activity")
		p.Done("7 PRs")
		p.Step("Repo History Merge Rate")
		p.Skip("not applicable")

		want := "step 1/3  Analysis Window ........... done (12 months)\n" +
			"step 2/3  Fetch GitHub Activity ..... done (7 PRs)\n" +
			"step 3/3  Repo History Merge Rate ... skipped (not applicable)\n"
		assert.Equal(t, want, buf.String(), "each step should be numbered and its result aligned in a column")
	})

	t.Run("Omits an empty detail", func(t *testing.T) {
		var buf bytes.Buffer
		p := New(&buf, 1)

		p.Step("Render Report")
		p.Done("")

		assert.Equal(t, "step 1/1  Render Report ............. done\n", buf.String(),
			"a step with no detail should not print empty parentheses")
	})

	t.Run("Pads step numbers to the width of the total", func(t *testing.T) {
		var buf bytes.Buffer
		p := New(&buf, 16)

		p.Step("Account Age")
		p.Done("")

		assert.Equal(t, "step  1/16  Account Age ............... done\n", buf.String(),
			"single-digit steps should be padded so the name column stays aligned")
	})
}

func TestStepperInterruption(t *testing.T) {
	t.Run("Problematic scenario", func(t *testing.T) {
		var buf bytes.Buffer
		p := New(&buf, 1)

		p.Step("Fetch GitHub Activity")
		// The standard logger writes here; see log.SetOutput in main.
		_, err := p.Write([]byte("Fetcher -> retrying after 2s: HTTP 502\n"))
		require.NoError(t, err, "writing a log line through the Stepper should not fail")
		p.Done("7 PRs")

		want := "step 1/1  Fetch GitHub Activity ..... \n" +
			"Fetcher -> retrying after 2s: HTTP 502\n" +
			"step 1/1  Fetch GitHub Activity ..... done (7 PRs)\n"
		assert.Equal(t, want, buf.String(),
			"an interrupting log line should end the open step line and the result should repeat the prefix")
	})

	t.Run("Repeats the prefix only once", func(t *testing.T) {
		var buf bytes.Buffer
		p := New(&buf, 1)

		p.Step("Fetch GitHub Activity")
		_, err := p.Write([]byte("first\n"))
		require.NoError(t, err, "the first interrupting line should not fail")
		_, err = p.Write([]byte("second\n"))
		require.NoError(t, err, "the second interrupting line should not fail")
		p.Done("")

		want := "step 1/1  Fetch GitHub Activity ..... \n" +
			"first\nsecond\n" +
			"step 1/1  Fetch GitHub Activity ..... done\n"
		assert.Equal(t, want, buf.String(),
			"consecutive interrupting lines should not each insert a blank line")
	})
}

func TestNilStepperIsSilent(t *testing.T) {
	var p *Stepper

	assert.NotPanics(t, func() {
		p.Step("Account Age")
		p.Done("3261 days")
		p.Skip("not applicable")
		_, _ = p.Write([]byte("ignored"))
	}, "a nil Stepper should be a no-op so callers without progress output need no nil checks")
}
