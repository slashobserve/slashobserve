import json
import re

import pytest
import yaml

from slashobserve_bench import orca

STRUCTURED = {
    "markdown": "The ad service returned errors after the flag changed.\n\nAt 09:44 UTC adFailure was switched on.\n10:02 UTC errors fell back.",
    "root_cause": {"component": "adFailure", "kind": "config_or_flag", "summary": "The adFailure flag made the ad service fail"},
    "onset": "2026-04-18T05:44:00-04:00",
    "impact": "14% of ad requests failed",
    "confidence": "high",
    "ruled_out": ["cart latency was normal", "payment errors were flat"],
    "next_steps": ["Switch adFailure off", "Alert on the ad error rate"],
    "no_incident": False,
}


def sections(report):
    return re.findall(r"^## (.+)$", report, flags=re.M)


def test_no_incident_writes_an_empty_report():
    assert orca.render_report({"markdown": "Nothing was wrong.", "no_incident": True}) == ""


def test_no_finding_writes_no_report():
    assert orca.report_for({"state": "failed", "finding": None}) is None
    assert orca.report_for({"state": "succeeded"}) is None


def test_report_has_the_four_sections_in_order():
    assert sections(orca.render_report(STRUCTURED)) == ["Summary", "Timeline", "5 Whys", "Remediation"]


def test_report_lays_out_the_structured_fields():
    report = orca.render_report(STRUCTURED)
    assert "adFailure (config or flag): The adFailure flag made the ad service fail" in report
    assert "Impact: 14% of ad requests failed" in report
    assert "2026-04-18 09:44 UTC - adFailure (config or flag): The adFailure flag made the ad service fail" in report
    assert "Problem statement:\n14% of ad requests failed" in report
    assert "Analysis:\nThe ad service returned errors" in report
    assert "Root cause:\nadFailure (config or flag)" in report
    assert "- cart latency was normal" in report
    assert "| Switch adFailure off | Root cause | Unclassified |" in report


def test_onset_is_written_in_utc_whatever_offset_it_carries():
    assert orca.clock("2026-04-18T05:44:00-04:00") == "2026-04-18 09:44 UTC"
    assert orca.clock("2026-04-18T09:44:30Z") == "2026-04-18 09:44 UTC"
    assert orca.clock("2026-04-18T11:44:00+02:00") == "2026-04-18 09:44 UTC"


def test_onset_without_an_offset_is_read_as_utc():
    assert orca.clock("2026-04-18T09:44:00") == "2026-04-18 09:44 UTC"


def test_unreadable_onset_is_kept_as_written():
    assert orca.clock(" around nine ") == "around nine"


def test_markdown_alone_still_makes_the_four_sections():
    report = orca.render_report({"markdown": "# Title\n\nCheckout failed because cart timed out.\n\nTimeline:\n- 09:44 UTC cart slowed\n- no times here"})
    assert sections(report) == ["Summary", "Timeline", "5 Whys", "Remediation"]
    assert "## Summary\n\nCheckout failed because cart timed out." in report
    assert "## Timeline\n\n09:44 UTC cart slowed\n" in report
    assert "no times here\n\n## 5 Whys" not in report
    assert "No remediation was suggested." in report


def test_markdown_without_any_time_says_so():
    assert "No incident start time was reported." in orca.render_report({"markdown": "Something failed."})


def test_structured_fields_may_be_null_as_the_api_returns_them():
    finding = {"markdown": "Cart is slow.", "root_cause": None, "onset": None, "impact": None, "ruled_out": [], "next_steps": [], "no_incident": False}
    assert sections(orca.render_report(finding)) == ["Summary", "Timeline", "5 Whys", "Remediation"]


def test_as_of_comes_from_the_task_metadata_in_utc():
    toml = '[metadata]\ncurrent = "2026-04-18T14:05:00-04:00"\n'
    assert orca.as_of(toml, "") == "2026-04-18T18:05:00Z"


def test_as_of_accepts_a_toml_datetime_and_drops_fractions():
    assert orca.as_of("[metadata]\ncurrent = 2026-04-18T18:05:00.250Z\n", "") == "2026-04-18T18:05:00Z"


@pytest.mark.parametrize(
    "stamp,expected",
    [("Apr 18, 2026 at 14:05", "2026-04-18T18:05:00Z"), ("Jan 12, 2026 at 09:30", "2026-01-12T14:30:00Z")],
)
def test_as_of_falls_back_to_the_instruction_in_eastern_time(stamp, expected):
    assert orca.as_of("[metadata]\ncategory = 'sre'\n", f"You are an SRE. The current time is {stamp} ET.\n") == expected


def test_as_of_needs_a_current_time():
    with pytest.raises(orca.UnsupportedTask):
        orca.as_of("[metadata]\n", "no time here")


def test_question_keeps_the_task_and_states_the_time_in_utc():
    text = orca.question("Task text.\n", "2026-04-18T18:05:00Z")
    assert text.startswith("Task text.\n\nNotes for this harness:\n")
    assert "2026-04-18T18:05:00Z (UTC)" in text
    assert "no_incident" in text


def test_start_body_is_an_investigation_as_of_the_current_time():
    assert orca.start_body("q", "2026-04-18T18:05:00Z") == {"kind": "investigate", "prompt": "q", "as_of": "2026-04-18T18:05:00Z"}


def test_ports_file_gives_the_store_urls():
    text = "export GRAFANA_URL=http://frontend-proxy:8080/grafana\nexport PROMETHEUS_URL=http://prometheus:9090\nexport OPENSEARCH_URL=http://opensearch:9200\n"
    assert orca.Endpoints.from_ports_file(text) == orca.Endpoints("http://prometheus:9090", "http://opensearch:9200")


def test_ports_file_without_a_store_is_refused():
    with pytest.raises(orca.UnsupportedTask, match="OPENSEARCH_URL"):
        orca.Endpoints.from_ports_file("export PROMETHEUS_URL=http://prometheus:9090\n")


def test_network_is_the_one_otel_compose_network():
    assert orca.otel_network(["bridge", "main_default", "otel-0123456789ab_default"]) == "otel-0123456789ab_default"


@pytest.mark.parametrize("names", [["bridge"], ["otel-0123456789ab_default", "otel-ba9876543210_default"]])
def test_network_must_be_unambiguous(names):
    with pytest.raises(orca.UnsupportedTask):
        orca.otel_network(names)


def test_datasources_read_metrics_logs_and_spans_without_jaeger():
    text = orca.datasources_yaml(orca.Endpoints("http://prometheus:9090", "http://opensearch:9200"))
    assert "type: jaeger" not in text
    assert '      url: "http://prometheus:9090"\n' in text
    assert text.count('url: "http://opensearch:9200"') == 2
    assert 'index: "otel-logs-*"' in text
    assert 'index: "jaeger-main-jaeger-span-*"' in text


def _entry(text, uid):
    found = yaml.safe_load(text)["datasources"]
    return next(entry for entry in found if entry["uid"] == uid)


def test_by_default_the_span_index_is_a_trace_source():
    text = orca.datasources_yaml(orca.Endpoints("http://prometheus:9090", "http://opensearch:9200"))
    spans = _entry(text, orca.SPANS_UID)["opensearch"]
    assert spans == {
        "url": "http://opensearch:9200",
        "index": "jaeger-main-jaeger-span-*",
        "timeField": "startTimeMillis",
        "spans": "jaeger",
    }
    assert _entry(text, orca.LOGS_UID)["opensearch"] == {"url": "http://opensearch:9200", "index": "otel-logs-*"}


def test_an_older_image_reads_the_spans_as_plain_documents_without_the_spans_field():
    text = orca.datasources_yaml(orca.Endpoints("http://prometheus:9090", "http://opensearch:9200"), span_index=False)
    spans = _entry(text, orca.SPANS_UID)["opensearch"]
    assert "spans" not in spans
    assert "spans:" not in text
    assert spans["timeField"] == "startTimeMillis"
    assert spans["fields"] == {"message": "operationName", "service": "process.serviceName", "traceId": "traceID", "spanId": "spanID"}


def test_span_index_is_an_option_that_defaults_on():
    assert orca.Options().span_index is True
    assert orca.Options(span_index="false").span_index is False
    with pytest.raises(ValueError):
        orca.Options(span_index="maybe")


def test_the_question_sends_the_agent_to_the_trace_tools_on_a_span_index():
    text = orca.question("Task text.", "2026-04-18T18:05:00Z")
    for tool in orca.TRACE_TOOLS:
        assert tool in text
    assert "spans are `traces`, which answers the trace tools" in text
    assert "logs_for_trace reads a trace's log lines from `logs`" in text
    assert "from its top operation down to the deepest span that failed" in text
    assert "all 32 hex characters" in text
    assert "Query DSL" not in text


def test_the_question_names_plain_span_documents_for_an_older_image():
    text = orca.question("Task text.", "2026-04-18T18:05:00Z", span_index=False)
    assert "spans are `traces` (Jaeger span documents in OpenSearch)." in text
    assert "trace_stats" not in text


def test_the_notes_say_how_the_spans_were_read():
    assert "spans: jaeger, so the trace tools answer on it" in orca.spans_note_line(True)
    assert "plain documents" in orca.spans_note_line(False)
    assert "--ak span_index=false" in orca.spans_mismatch_hint(True)
    assert orca.spans_mismatch_hint(False) == ""


def test_subset_is_four_medium_four_hard_and_two_controls():
    incidents = [task for task in orca.SUBSET if not task.control]
    assert sorted(task.tier for task in incidents) == ["hard"] * 4 + ["medium"] * 4
    assert len([task for task in orca.SUBSET if task.control]) == 2
    assert len({task.package for task in orca.SUBSET}) == 10
    assert all(re.fullmatch(r"[0-9a-f]{16}", task.package) for task in orca.SUBSET)


def test_include_flags_name_every_task_by_a_glob_that_ignores_the_org_prefix():
    flags = orca.include_flags()
    assert flags[:2] == ["--include-task-name", f"*{orca.SUBSET[0].package}"]
    assert len(flags) == 20


def test_options_refuse_an_unknown_name_and_a_cap_without_a_ledger():
    with pytest.raises(ValueError):
        orca.Options(bogus=1)
    with pytest.raises(ValueError, match="go together"):
        orca.Options(spend_cap_usd=5)
    assert orca.Options(investigation_calls="30").investigation_calls == 30


def test_options_take_the_loop_settings_and_refuse_values_the_server_refuses():
    options = orca.Options(investigation_loop="false", investigation_calls_max="40", finding_grounding="enforce")
    assert (options.investigation_loop, options.investigation_calls_max, options.finding_grounding) == (False, 40, "enforce")
    for bad in ({"investigation_calls_max": 61}, {"investigation_calls_max": 11}, {"finding_grounding": "strict"}, {"investigation_calls": 30, "investigation_calls_max": 20}):
        with pytest.raises(ValueError):
            orca.Options(**bad)


def test_code_context_is_a_module_and_function():
    assert orca.load_code_context("json:dumps") is json.dumps
    with pytest.raises(ValueError):
        orca.load_code_context("json")


@pytest.mark.parametrize(
    ("value", "mode"), [(None, "codeserver"), ("off", "off"), ("my.module:register", "hook")]
)
def test_code_context_picks_the_code_server_off_or_a_hook(value, mode):
    assert orca.Options(code_context=value).code_mode == mode


def test_a_code_context_that_is_neither_off_nor_a_hook_is_refused():
    with pytest.raises(ValueError, match="<module>:<function>, or off"):
        orca.Options(code_context="none")


def test_source_names_default_to_the_new_ones_and_take_only_the_two_namings():
    assert orca.Options().names.setting == "SLASHOBSERVE_MCP_SOURCES_FILE"
    assert orca.Options(source_names="context").names.key == "contextSources"
    with pytest.raises(ValueError):
        orca.Options(source_names="both")


@pytest.mark.parametrize(
    ("option", "key", "setting", "file"),
    [
        ("mcp", "mcpSources", "SLASHOBSERVE_MCP_SOURCES_FILE", "mcp-sources.yaml"),
        ("context", "contextSources", "SLASHOBSERVE_CONTEXT_SOURCES_FILE", "context-sources.yaml"),
    ],
)
def test_the_code_source_is_provisioned_under_either_naming(option, key, setting, file):
    names = orca.SOURCE_NAMES[option]
    text = orca.code_files(names)[file]
    assert text.splitlines()[:2] == [f"{key}:", "  - uid: orca-code"]
    assert '    url: "http://codeserver.orca.internal:8080/mcp"' in text
    assert "    allowPrivate: true" in text
    assert '    approve: ["list_dir", "read_file", "grep"]' in text
    assert orca.code_settings(names) == (
        (setting, f"/etc/slashobserve/{file}"),
        ("SLASHOBSERVE_OUTBOUND_DOMAINS", "codeserver.orca.internal"),
    )


def test_the_code_server_runs_read_only_on_the_trial_network(tmp_path):
    args = orca.codeserver_args("code-1", "otel-0123456789ab_default", tmp_path, "slashobserve-codeserver:local")
    assert args[:2] == ["run", "-d"]
    pairs = list(zip(args, args[1:]))
    assert ("--network", "otel-0123456789ab_default") in pairs
    assert ("--network-alias", "codeserver.orca.internal") in pairs
    assert ("--label", "slashobserve-bench") in pairs
    assert ("-v", f"{tmp_path.resolve()}:/code:ro") in pairs
    assert "--read-only" in args
    assert args[-5:] == ["slashobserve-codeserver:local", "-root", "/code", "-addr", ":8080"]


def test_code_tools_are_ready_only_when_all_three_are_approved():
    approved = [{"name": name, "state": "approved"} for name in orca.CODE_TOOLS]
    assert orca.code_tools_problem(approved) is None
    assert orca.code_tools_problem(approved[:2] + [{"name": "grep", "state": "refused"}]) == "grep is refused"
    assert orca.code_tools_problem([]) == "list_dir is missing, read_file is missing, grep is missing"


def test_the_mismatch_hint_names_the_other_naming():
    assert "pass --ak source_names=context" in orca.code_mismatch_hint(orca.SOURCE_NAMES["mcp"])
    assert "pass --ak source_names=mcp" in orca.code_mismatch_hint(orca.SOURCE_NAMES["context"])


def test_the_notes_say_which_code_context_applied():
    assert orca.code_note_line(orca.Options()) == (
        "code context: the code server slashobserve-codeserver:local over a read-only copy of /app/opentelemetry-demo, "
        "provisioned as the MCP source orca-code with list_dir, read_file, grep approved, "
        "through SLASHOBSERVE_MCP_SOURCES_FILE with the key mcpSources"
    )
    assert orca.code_note_line(orca.Options(code_context="x:y")) == "code context: registered through x:y"
    assert orca.code_note_line(orca.Options(code_context="off")) == "code context: off, the run had no source code (--ak code_context=off)"


def test_the_question_points_at_the_code_and_asks_for_the_flag_key():
    text = orca.question("Task text.", "2026-04-18T18:05:00Z", orca.code_note("codeserver"))
    assert "MCP source `orca-code` (tools list_dir, read_file and grep)" in text
    assert "feature flag" in text
    assert "spelled exactly as the source spells it" in text
    assert "the code source registered for this run" in orca.code_note("hook")
    assert orca.code_note("off") is None
    assert "feature flag" not in orca.question("Task text.", "2026-04-18T18:05:00Z")


@pytest.mark.parametrize("span_index", [True, False])
def test_the_question_stays_within_the_server_prompt_limit(span_index):
    note = orca.code_note("codeserver")
    empty = len(orca.question("", "2026-04-18T18:05:00Z", note, span_index=span_index))
    assert len(orca.question("x" * (orca.PROMPT_LIMIT - empty), "2026-04-18T18:05:00Z", note, span_index=span_index)) == orca.PROMPT_LIMIT
    with pytest.raises(orca.UnsupportedTask, match="over the server's 4000"):
        orca.question("x" * (orca.PROMPT_LIMIT - empty + 1), "2026-04-18T18:05:00Z", note, span_index=span_index)


def test_replay_fixture_swaps_only_the_prompt():
    fixture = orca.replay_fixture({"prompt": "x", "turns": []}, "  question \n")
    assert fixture == {"prompt": "question", "turns": []}


def test_notes_are_one_line_each(tmp_path):
    orca.write_notes(tmp_path / "agent", ["a", "b"])
    assert (tmp_path / "agent" / "notes.txt").read_text() == "a\nb\n"


def test_replay_fixture_command_writes_the_fixture_for_a_task_folder(tmp_path):
    task = tmp_path / "abc"
    task.mkdir()
    (task / "instruction.md").write_text("The current time is Apr 18, 2026 at 14:05 ET.\nbody\n")
    (task / "task.toml").write_text("[metadata]\ncategory = 'sre'\n")
    template = tmp_path / "template.json"
    template.write_text('{"prompt": "x", "turns": []}')
    assert orca.main(["replay-fixture", "--task-dir", str(task), "--template", str(template), "--out", str(tmp_path / "out")]) == 0
    fixture = json.loads((tmp_path / "out" / "abc.json").read_text())
    assert "2026-04-18T18:05:00Z" in fixture["prompt"]
    assert "MCP source `orca-code`" in fixture["prompt"]
    assert fixture["turns"] == []
    args = ["replay-fixture", "--task-dir", str(task), "--template", str(template), "--out", str(tmp_path / "off"), "--code", "off"]
    assert orca.main(args) == 0
    assert "feature flag" not in json.loads((tmp_path / "off" / "abc.json").read_text())["prompt"]
    assert "trace_error_origins" in fixture["prompt"]
    args = ["replay-fixture", "--task-dir", str(task), "--template", str(template), "--out", str(tmp_path / "plain"), "--span-index", "false"]
    assert orca.main(args) == 0
    assert "trace_error_origins" not in json.loads((tmp_path / "plain" / "abc.json").read_text())["prompt"]


def test_naive_task_time_is_eastern_like_the_instruction_time():
    assert orca.as_of('[metadata]\ncurrent = "2026-04-18T14:05:00"\n', "") == "2026-04-18T18:05:00Z"
    assert orca.as_of("[metadata]\ncurrent = 2026-01-12T09:30:00\n", "") == "2026-01-12T14:30:00Z"


def test_instruction_time_may_have_single_digit_day_and_hour():
    assert orca.as_of("[metadata]\n", "The current time is Apr 8, 2026 at 9:05 ET.") == "2026-04-08T13:05:00Z"


def test_headings_in_the_answer_stay_below_the_four_sections():
    finding = {"markdown": "# Findings\n\nIntro.\n\n## Evidence\n\n```\n# not a heading\n```\n\n###### Deep\n", "no_incident": False}
    report = orca.render_report(finding)
    assert sections(report) == ["Summary", "Timeline", "5 Whys", "Remediation"]
    assert "### Findings" in report
    assert "#### Evidence" in report
    assert "# not a heading" in report
    assert "###### Deep" in report
    assert "\n# Findings" not in report


def test_a_heading_with_a_time_does_not_become_a_timeline_heading():
    report = orca.render_report({"markdown": "## 09:44 UTC cart slowed\n\nText."})
    assert "## Timeline\n\n09:44 UTC cart slowed\n" in report


def test_compose_project_is_named_the_way_harbor_names_it():
    assert orca.compose_project("Trial__ABC.1") == "trial__abc-1"
    assert orca.compose_project("_x") == "0_x"
    assert orca.compose_project("-x") == "0-x"
