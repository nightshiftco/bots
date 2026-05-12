package slackbot

import "testing"

func TestSlackifyMarkdown_BoldAndLinks(t *testing.T) {
	in := "**Sharing does happen on the backend — the bug is in the nightshift API, not the UI.**\n" +
		"\n" +
		"**What I did**\n" +
		"- Traced the path: dialog → shareArtifact() in `lib/api.ts` → app/api/artifacts/[id]/share/route.ts\n" +
		"- Filed [nightshiftco/nightshift#160](https://github.com/nightshiftco/nightshift/issues/160) with the trace.\n"

	want := "*Sharing does happen on the backend — the bug is in the nightshift API, not the UI.*\n" +
		"\n" +
		"*What I did*\n" +
		"• Traced the path: dialog → shareArtifact() in `lib/api.ts` → app/api/artifacts/[id]/share/route.ts\n" +
		"• Filed <https://github.com/nightshiftco/nightshift/issues/160|nightshiftco/nightshift#160> with the trace."

	got := SlackifyMarkdown(in)
	if got != want {
		t.Errorf("mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestSlackifyMarkdown_PreservesCode(t *testing.T) {
	in := "Look at `**not bold**` inside code, then **really bold** outside."
	want := "Look at `**not bold**` inside code, then *really bold* outside."
	if got := SlackifyMarkdown(in); got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestSlackifyMarkdown_FencedCodeUntouched(t *testing.T) {
	in := "before\n```\n**still markdown here**\n[link](http://x)\n```\nafter **bold**"
	want := "before\n```\n**still markdown here**\n[link](http://x)\n```\nafter *bold*"
	if got := SlackifyMarkdown(in); got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestSlackifyMarkdown_Headers(t *testing.T) {
	in := "# Top\n## Sub\nbody"
	want := "*Top*\n*Sub*\nbody"
	if got := SlackifyMarkdown(in); got != want {
		t.Errorf("got %q want %q", got, want)
	}
}
