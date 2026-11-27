import pytest

from slashobserve_bench import server
from slashobserve_bench.loop_options import LoopOptions

OLDER_IMAGE_DEFAULTS = {"SLASHOBSERVE_AGENT_INVESTIGATION_LOOP": "false", "SLASHOBSERVE_AGENT_FINDING_GROUNDING": "off"}
NEWER_IMAGE_DEFAULTS = {"SLASHOBSERVE_AGENT_INVESTIGATION_LOOP": "true", "SLASHOBSERVE_AGENT_FINDING_GROUNDING": "enforce", "SLASHOBSERVE_AGENT_INVESTIGATION_CALLS_MAX": "40"}


def env_of(options: LoopOptions) -> dict[str, str]:
    config = server.ServerConfig(image="i", network="n", llm_provider="replay", **options.server_settings())
    return server.server_env(config, "postgres://db")


def ran_with(image_defaults: dict[str, str], options: LoopOptions) -> dict[str, str]:
    return {**image_defaults, **{name: value for name, value in env_of(options).items() if name.startswith("SLASHOBSERVE_AGENT_")}}


def test_unset_options_send_the_newer_server_defaults_so_either_image_runs_with_what_is_recorded():
    options = LoopOptions()
    recorded = options.recorded_loop_settings()
    assert recorded == {"investigation_calls": "server default", "investigation_loop": True, "investigation_calls_max": 40, "finding_grounding": "enforce", "investigation_brief": "on"}
    for image in (OLDER_IMAGE_DEFAULTS, NEWER_IMAGE_DEFAULTS):
        env = ran_with(image, options)
        assert env["SLASHOBSERVE_AGENT_INVESTIGATION_LOOP"] == "true"
        assert env["SLASHOBSERVE_AGENT_INVESTIGATION_CALLS_MAX"] == "40"
        assert env["SLASHOBSERVE_AGENT_FINDING_GROUNDING"] == "enforce"
        assert env["SLASHOBSERVE_AGENT_INVESTIGATION_BRIEF"] == "on"
        assert "SLASHOBSERVE_AGENT_INVESTIGATION_CALLS" not in env


def test_the_older_baseline_is_sent_whole_and_recorded_as_sent():
    options = LoopOptions(investigation_calls=12, investigation_loop=False, finding_grounding="off", investigation_brief="off")
    assert options.recorded_loop_settings() == {"investigation_calls": 12, "investigation_loop": False, "investigation_calls_max": 40, "finding_grounding": "off", "investigation_brief": "off"}
    for image in (OLDER_IMAGE_DEFAULTS, NEWER_IMAGE_DEFAULTS):
        env = ran_with(image, options)
        sent = ("INVESTIGATION_CALLS", "INVESTIGATION_LOOP", "FINDING_GROUNDING", "INVESTIGATION_BRIEF")
        assert tuple(env[f"SLASHOBSERVE_AGENT_{name}"] for name in sent) == ("12", "false", "off", "off")


def test_given_values_win_over_the_adapter_defaults():
    options = LoopOptions(investigation_calls=20, investigation_loop=False, investigation_calls_max=24, finding_grounding="report", investigation_brief="off")
    assert options.server_settings() == {"investigation_calls": 20, "investigation_loop": False, "investigation_calls_max": 24, "finding_grounding": "report", "investigation_brief": "off"}


def test_calls_max_follows_calls_above_the_default_ceiling():
    assert LoopOptions(investigation_calls=50).effective_calls_max == 50
    assert LoopOptions(investigation_calls=20).effective_calls_max == 40


@pytest.mark.parametrize("kwargs", [{"investigation_calls_max": 11}, {"investigation_calls": 30, "investigation_calls_max": 20}])
def test_a_ceiling_below_the_starting_limit_of_either_image_is_refused(kwargs):
    with pytest.raises(ValueError, match="investigation_calls_max"):
        LoopOptions(**kwargs)


def test_the_loop_note_names_what_unset_calls_mean_on_each_image():
    note = LoopOptions().loop_note()
    assert note.startswith("investigation loop: true, calls max 40, calls server default")
    assert "by task type" in note and "12 on an older one" in note
    assert "calls 30, preflight sweep: on;" in LoopOptions(investigation_calls=30).loop_note()
    assert "preflight sweep: off;" in LoopOptions(investigation_brief="off").loop_note()


@pytest.mark.parametrize("value", ["true", "maybe", ""])
def test_the_preflight_sweep_takes_only_on_or_off(value):
    with pytest.raises(ValueError):
        LoopOptions(investigation_brief=value)
