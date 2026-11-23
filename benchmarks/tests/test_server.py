import re
from pathlib import Path

import pytest

from slashobserve_bench import server
from slashobserve_bench.o11y import Stores

STORES = Stores("http://stack:9090", "http://stack:3100", "http://stack:3200")


def config(**changes):
    values = {"image": "slashobserve-server:local", "network": "o11y-bench-shared", "llm_provider": "replay"}
    return server.ServerConfig(**{**values, **changes})


def test_key_has_the_service_account_key_shape():
    key = server.new_key()
    assert re.fullmatch(r"sok_[A-Za-z0-9_-]{43}", key)
    assert key != server.new_key()


def test_service_accounts_file_names_an_admin_with_a_key_file():
    text = server.service_accounts_yaml("/etc/slashobserve/benchmark.key")
    assert text == (
        "serviceAccounts:\n"
        "  - name: benchmark\n"
        "    role: admin\n"
        '    keyFile: "/etc/slashobserve/benchmark.key"\n'
    )


def test_datasources_file_points_each_store_at_the_stack():
    text = server.datasources_yaml(STORES)
    assert text.startswith("datasources:\n")
    for uid, url in (("prometheus", "http://stack:9090"), ("loki", "http://stack:3100"), ("tempo", "http://stack:3200")):
        assert f"  - uid: {uid}\n    type: {uid}\n" in text
        assert f'    {uid}:\n      url: "{url}"\n' in text


def test_replay_env_sets_fixture_dir_and_no_model(tmp_path):
    env = server.server_env(config(replay_dir=tmp_path), "postgres://db")
    assert env["SLASHOBSERVE_LLM_PROVIDER"] == "replay"
    assert env["SLASHOBSERVE_LLM_REPLAY_DIR"] == server.FIXTURES_DIR
    assert "SLASHOBSERVE_LLM_MODEL" not in env
    assert "SLASHOBSERVE_AGENT_INVESTIGATION_CALLS" not in env
    assert "SLASHOBSERVE_AGENT_RUN_MAX_COST_USD" not in env


def test_budget_and_cost_settings_reach_the_server():
    env = server.server_env(
        config(llm_provider="anthropic", llm_model="claude-haiku-5-5", investigation_calls=30, run_max_cost_usd=0.5),
        "postgres://db",
    )
    assert env["SLASHOBSERVE_AGENT_INVESTIGATION_CALLS"] == "30"
    assert env["SLASHOBSERVE_AGENT_RUN_MAX_COST_USD"] == "0.5"
    assert env["SLASHOBSERVE_LLM_MODEL"] == "claude-haiku-5-5"


def test_server_command_publishes_a_free_loopback_port_and_joins_the_network(tmp_path):
    args = server.server_args("name", config(), {"A": "b"}, tmp_path, None)
    assert args[args.index("--network") + 1] == "o11y-bench-shared"
    assert args[args.index("-p") + 1] == "127.0.0.1::8080"
    assert args[-1] == "slashobserve-server:local"
    assert "--env-file" not in args


def test_llm_key_travels_in_an_env_file_never_on_the_command_line(tmp_path):
    env_file = tmp_path / "llm.env"
    args = server.server_args("name", config(llm_provider="anthropic"), {"A": "b"}, tmp_path, env_file)
    assert args[args.index("--env-file") + 1] == str(env_file)
    assert not any("API_KEY" in arg for arg in args)


def test_replay_fixtures_are_mounted_read_only(tmp_path):
    args = server.server_args("name", config(replay_dir=tmp_path), {}, Path("/cfg"), None)
    assert f"{tmp_path.resolve()}:{server.FIXTURES_DIR}:ro" in args


def test_postgres_runs_in_memory_on_the_shared_network():
    args = server.postgres_args("pg", "o11y-bench-shared", "secret")
    assert args[args.index("--tmpfs") + 1] == "/var/lib/postgresql"
    assert args[args.index("--network") + 1] == "o11y-bench-shared"
    assert args[-1] == server.POSTGRES_IMAGE


@pytest.mark.parametrize(("output", "port"), [("127.0.0.1:49153\n", 49153), ("0.0.0.0:32768\n[::]:32768\n", 32768)])
def test_published_port_is_parsed(output, port):
    assert server.parse_published_port(output) == port


@pytest.mark.parametrize("output", ["", "no port\n"])
def test_unparsable_port_is_an_error(output):
    with pytest.raises(server.StackError):
        server.parse_published_port(output)


def test_stack_writes_config_files_readable_by_the_nonroot_container(tmp_path):
    stack = server.Stack(config(), STORES, tmp_path, None)
    config_dir = stack._write_config()
    try:
        assert (config_dir / "benchmark.key").read_text() == stack.key + "\n"
        assert "benchmark.key" in (config_dir / "service-accounts.yaml").read_text()
        assert config_dir.stat().st_mode & 0o755 == 0o755
        assert (config_dir / "benchmark.key").stat().st_mode & 0o044 == 0o044
        assert stack._write_env_file() is None
    finally:
        stack._workdir.cleanup()


def test_llm_env_file_is_private(tmp_path):
    stack = server.Stack(config(llm_provider="anthropic"), STORES, tmp_path, "sk-test")
    try:
        path = stack._write_env_file()
        assert path.stat().st_mode & 0o077 == 0
        assert path.read_text() == "SLASHOBSERVE_LLM_API_KEY=sk-test\n"
    finally:
        stack._workdir.cleanup()
