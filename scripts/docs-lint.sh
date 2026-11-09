#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
DOCS_DIR="${DOCS_DIR:-docs}"

files=()
while IFS= read -r file; do
  files+=("$file")
done < <(find "$DOCS_DIR" -type f -name '*.md' | LC_ALL=C sort)

if [[ ${#files[@]} -eq 0 ]]; then
  echo "docs-lint: no Markdown files under $DOCS_DIR" >&2
  exit 1
fi

perl -CSD - "${files[@]}" <<'PERL'
use strict;
use warnings;
use File::Basename qw(dirname);

use constant MAX_PARAGRAPH_LINES => 8;
use constant MAX_BODY_LINES_WITHOUT_SECTION => 60;

my $violations = 0;
my ($state, $fence, %front);
my ($run, $run_start, $in_list, $body_lines, $has_section);

sub report_at {
  my ($line, $rule) = @_;
  print "$ARGV:$line: $rule\n";
  $violations++;
}

sub report {
  my ($rule) = @_;
  report_at($., $rule);
}

sub end_paragraph {
  report_at($run_start, "paragraph of $run lines (max " . MAX_PARAGRAPH_LINES . ")")
    if $run > MAX_PARAGRAPH_LINES;
  $run = 0;
}

sub prose {
  my ($text) = @_;
  $text =~ s/(`+).*?\1//g;
  $text =~ s/"[^"]*"//g;
  1 while $text =~ s/\([^()]*\)//g;
  return $text;
}

sub check_link {
  my ($target) = @_;
  $target =~ s/\s.*//;
  $target =~ s/^<(.*)>$/$1/;
  return if $target =~ m{^(?:[a-z][a-z0-9+.-]*:|#|/)}i;
  $target =~ s/[#?].*//;
  return unless $target =~ /\.(?:md|png|jpg|svg)$/i;
  my $path = dirname($ARGV) . "/" . $target;
  report("broken link $target") unless -f $path;
}

sub start_file {
  $state = "start";
  $fence = "";
  %front = ();
  ($run, $run_start, $in_list, $body_lines, $has_section) = (0, 0, 0, 0, 0);
}

sub end_file {
  end_paragraph();
  if ($state eq "front") {
    report("unclosed front matter");
  } else {
    for my $key ("title", "description") {
      next if $front{$key};
      report_at(1, "front matter has no $key");
    }
    report_at(1, "no ## heading in a page of $body_lines lines (max " . MAX_BODY_LINES_WITHOUT_SECTION . ")")
      if $body_lines > MAX_BODY_LINES_WITHOUT_SECTION && !$has_section;
  }
}

start_file();
while (my $line = <>) {
  chomp $line;
  report("em or en dash") if $line =~ /[\x{2013}\x{2014}]/;

  if ($state eq "start") {
    $state = $line eq "---" ? "front" : "body";
    next if $state eq "front";
  }
  if ($state eq "front") {
    if ($line eq "---") {
      $state = "body";
    } elsif ($line =~ /^(title|description):\s*\S/) {
      $front{$1} = 1;
    }
    next;
  }

  $body_lines++ if $line !~ /^\s*$/;

  if ($fence ne "") {
    $fence = "" if $line =~ /^\s*\Q$fence\E\s*$/;
    next;
  }
  if ($line =~ /^\s*(`{3,}|~{3,})/) {
    $fence = $1;
    end_paragraph();
    next;
  }

  report("level 1 heading") if $line =~ /^# /;
  $has_section = 1 if $line =~ /^## /;

  my $links = $line;
  $links =~ s/(`+).*?\1//g;
  report("image without alt text") while $links =~ /!\[\s*\]\(/g;
  check_link($1) while $links =~ /\]\(([^)]+)\)/g;
  check_link($1) if $links =~ /^\s*\[[^\]]+\]:\s*(\S+)/;

  my $list_item = $line =~ /^\s*(?:[-*+]|\d+[.)])\s/;
  $in_list = $list_item || ($in_list && $line =~ /^\s+\S/);
  if ($line =~ /^\s*$/ || $in_list || $line =~ /^\s*(?:\||#|>)/) {
    end_paragraph();
  } else {
    $run_start = $. unless $run;
    $run++;
  }

  next if $line =~ /^\s*$/;
  next if $list_item;
  next if $line =~ /^\s*\|/;
  next if $line =~ /^\s*#/;
  report("more than one sentence on the line") if prose($line) =~ /\.\s+[A-Z]/;
}
continue {
  if (eof) {
    end_file();
    close ARGV;
    start_file();
  }
}

exit($violations > 0 ? 1 : 0);
PERL
