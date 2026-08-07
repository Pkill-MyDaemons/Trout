package main

import "testing"

func TestStripThinkBlocksRemovesClosedBlock(t *testing.T) {
	in := "<think>hmm, let me consider the options...\nokay.</think>\n\nHere is the answer."
	got := stripThinkBlocks(in)
	want := "Here is the answer."
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestStripThinkBlocksRemovesMultipleBlocks(t *testing.T) {
	in := "<think>first</think>Answer part one.<think>second</think> Part two."
	got := stripThinkBlocks(in)
	want := "Answer part one. Part two."
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestStripThinkBlocksHandlesUnterminatedTrailingBlock(t *testing.T) {
	in := "Some answer text.\n<think>this reasoning got cut off mid"
	got := stripThinkBlocks(in)
	want := "Some answer text."
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestStripThinkBlocksIsCaseInsensitive(t *testing.T) {
	in := "<THINK>reasoning</THINK>Final answer."
	got := stripThinkBlocks(in)
	want := "Final answer."
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestStripThinkBlocksNoOpWithoutThinkTag(t *testing.T) {
	in := "Plain answer, nothing to strip."
	if got := stripThinkBlocks(in); got != in {
		t.Fatalf("got %q, want unchanged %q", got, in)
	}
}

func TestParseAgentResponseStripsThinkBeforeFileTrailer(t *testing.T) {
	raw := "<think>planning the file write...</think>\n" +
		"Done, wrote the file.\n\n" +
		"<!-- task-agent-files\n" +
		"projects/demo/main.go\n" +
		"-->"
	body, files := parseAgentResponse(raw)
	if body != "Done, wrote the file." {
		t.Fatalf("unexpected body: %q", body)
	}
	if len(files) != 1 || files[0] != "projects/demo/main.go" {
		t.Fatalf("unexpected files: %v", files)
	}
}
