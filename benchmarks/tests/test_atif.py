import copy

from harbor.models.trajectories import Trajectory

from slashobserve_bench import atif


def build(request, model_name="anthropic/claude-haiku-5-5"):
    return atif.build_trajectory(
        request, "Why did checkout fail?", agent_name="slashobserve", agent_version="0.1.0", model_name=model_name
    )


def test_trajectory_is_valid_atif(finished_request):
    Trajectory.model_validate(build(finished_request))


def test_trajectory_has_user_tool_and_answer_steps(finished_request):
    steps = build(finished_request)["steps"]
    assert [step["source"] for step in steps] == ["user", "agent", "agent", "agent"]
    assert [step["step_id"] for step in steps] == [1, 2, 3, 4]
    assert steps[0]["message"] == "Why did checkout fail?"
    assert steps[1]["tool_calls"][0]["function_name"] == "list_labels"
    assert steps[2]["observation"]["results"][0]["content"] == "error: The query was refused"


def test_last_step_is_the_finding_without_tool_calls(finished_request):
    last = build(finished_request)["steps"][-1]
    assert last["message"] == finished_request["finding"]["markdown"]
    assert "tool_calls" not in last
    assert last["model_name"] == "anthropic/claude-haiku-5-5"


def test_final_metrics_count_cache_in_prompt_tokens(finished_request):
    metrics = build(finished_request)["final_metrics"]
    assert metrics["total_prompt_tokens"] == 1350
    assert metrics["total_completion_tokens"] == 200
    assert metrics["total_cached_tokens"] == 300
    assert metrics["total_cost_usd"] == 0.0123
    assert metrics["total_steps"] == 4
    assert metrics["extra"]["as_of"] == "2026-11-20T10:00:00Z"


def test_failed_run_ends_with_a_reason_instead_of_an_answer(finished_request):
    request = copy.deepcopy(finished_request)
    request.update(state="failed", failure_code="budget_exhausted", error="cost limit reached", finding=None)
    trajectory = build(request)
    Trajectory.model_validate(trajectory)
    message = trajectory["steps"][-1]["message"]
    assert "failed (budget_exhausted)" in message
    assert "cost limit reached" in message


def test_succeeded_run_without_a_finding_says_so(finished_request):
    request = {**finished_request, "finding": None}
    assert atif.answer_text(request) == "The run finished without saving a finding."


def test_run_with_no_usage_and_no_steps(finished_request):
    request = {**finished_request, "usage": None, "steps": [], "finished_at": None}
    trajectory = build(request, model_name=None)
    Trajectory.model_validate(trajectory)
    assert len(trajectory["steps"]) == 2
    assert trajectory["final_metrics"]["total_prompt_tokens"] == 0
    assert trajectory["final_metrics"]["total_cost_usd"] is None
    assert "model_name" not in trajectory["agent"]


def test_answer_leads_with_the_structured_fields_when_the_finding_has_them(finished_request):
    finding = {
        **finished_request["finding"],
        "root_cause": {"component": "checkout", "kind": "deploy", "summary": "Version 42 added a retry."},
        "onset": "2026-11-20T10:00:00Z",
        "impact": "12% of orders failed.",
        "confidence": "high",
        "ruled_out": ["the database", "the cache"],
        "next_steps": ["Roll back checkout."],
        "no_incident": False,
    }
    answer = atif.answer_text({**finished_request, "finding": finding})
    assert answer.splitlines()[:6] == [
        "Root cause: checkout (deploy) - Version 42 added a retry.",
        "Onset: 2026-11-20T10:00:00Z",
        "Impact: 12% of orders failed.",
        "Confidence: high",
        "Ruled out: the database; the cache",
        "Next steps: Roll back checkout.",
    ]
    assert answer.endswith(finding["markdown"])


def test_answer_is_the_markdown_alone_when_no_field_is_set(finished_request):
    finding = {**finished_request["finding"], "root_cause": None, "onset": None, "ruled_out": [], "next_steps": [], "no_incident": False}
    assert atif.answer_text({**finished_request, "finding": finding}) == finding["markdown"]


def test_answer_has_one_line_per_cause_with_its_band_and_whether_it_is_stated(finished_request):
    def entry(name, mechanism, **fields):
        base = {"component": {"kind": "service", "name": name, "namespace": None}, "kind": "config_or_flag", "mechanism": mechanism, "trigger": None, "relation": "independent", "knock_on_of": None, "band": "high", "stated": True}
        return {**base, **fields}

    causes = [
        entry("ad", "The flag made ad fail.", trigger={"kind": "feature_flag", "key": "adFailure", "value": "on"}),
        entry("checkout", "Checkout failed on calls to ad.", kind="dependency", relation="knock_on", knock_on_of=0, band="medium", component={"kind": "deployment", "name": "checkout", "namespace": "shop"}),
        entry("payment", "Payment may be slow.", band="low", stated=False),
        entry("cart", "Unscored.", band=None),
    ]
    finding = {**finished_request["finding"], "root_cause": {"component": "ad", "kind": "config_or_flag", "summary": "x"}, "causes": causes, "no_incident": False}
    assert atif.structured_lines(finding) == [
        "Cause 1 (high confidence, stated): ad (config_or_flag) - The flag made ad fail. Trigger: adFailure=on",
        "Cause 2 (medium confidence, stated): knock-on of cause 1, shop/checkout (dependency) - Checkout failed on calls to ad.",
        "Cause 3 (low confidence, unconfirmed): payment (config_or_flag) - Payment may be slow.",
        "Cause 4 (stated): cart (config_or_flag) - Unscored.",
    ]


def test_an_older_finding_without_causes_keeps_the_root_cause_line(finished_request):
    finding = {**finished_request["finding"], "root_cause": {"component": "checkout", "kind": "deploy", "summary": "Version 42."}, "causes": []}
    assert atif.structured_lines(finding)[0] == "Root cause: checkout (deploy) - Version 42."


def test_a_finding_with_no_incident_says_so_before_the_markdown(finished_request):
    finding = {**finished_request["finding"], "no_incident": True}
    assert atif.answer_text({**finished_request, "finding": finding}).startswith("No incident found.\n\n")
