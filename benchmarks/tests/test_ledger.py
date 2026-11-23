from concurrent.futures import ThreadPoolExecutor

import pytest

from slashobserve_bench.ledger import SpendCapReached, SpendLedger


def test_empty_ledger_has_room(tmp_path):
    ledger = SpendLedger(tmp_path / "ledger.jsonl", 1.0)
    assert ledger.total() == 0
    ledger.ensure_room()


def test_record_returns_the_running_total(tmp_path):
    ledger = SpendLedger(tmp_path / "ledger.jsonl", 1.0)
    assert ledger.record("a", 0.25) == pytest.approx(0.25)
    assert ledger.record("b", 0.5) == pytest.approx(0.75)


def test_reaching_the_cap_refuses_the_next_trial(tmp_path):
    ledger = SpendLedger(tmp_path / "ledger.jsonl", 1.0)
    ledger.record("a", 1.0)
    with pytest.raises(SpendCapReached) as caught:
        ledger.ensure_room()
    assert caught.value.spent_usd == pytest.approx(1.0)
    assert "$1.00" in str(caught.value)


def test_a_second_ledger_object_sees_the_same_file(tmp_path):
    path = tmp_path / "nested" / "ledger.jsonl"
    SpendLedger(path, 5.0).record("a", 2.0)
    assert SpendLedger(path, 5.0).total() == pytest.approx(2.0)


def test_concurrent_records_lose_nothing(tmp_path):
    ledger = SpendLedger(tmp_path / "ledger.jsonl", 1000.0)
    with ThreadPoolExecutor(8) as pool:
        list(pool.map(lambda n: ledger.record(f"t{n}", 1.0), range(40)))
    assert ledger.total() == pytest.approx(40.0)
