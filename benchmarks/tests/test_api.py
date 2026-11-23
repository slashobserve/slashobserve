import pytest

from slashobserve_bench.api import RunTimeout, wait_for_terminal


class Fake:
    def __init__(self, states):
        self.states = list(states)
        self.now = 0.0
        self.slept = []

    def fetch(self):
        return {"state": self.states.pop(0) if len(self.states) > 1 else self.states[0]}

    def clock(self):
        return self.now

    def sleep(self, seconds):
        self.slept.append(seconds)
        self.now += seconds


def test_returns_the_first_terminal_request():
    fake = Fake(["queued", "running", "succeeded"])
    request = wait_for_terminal(fake.fetch, 60, 1.0, fake.clock, fake.sleep)
    assert request["state"] == "succeeded"
    assert fake.slept == [1.0, 1.0]


@pytest.mark.parametrize("state", ["failed", "canceled"])
def test_failed_and_canceled_are_terminal(state):
    fake = Fake([state])
    assert wait_for_terminal(fake.fetch, 60, 1.0, fake.clock, fake.sleep)["state"] == state


def test_times_out_while_running():
    fake = Fake(["running"])
    with pytest.raises(RunTimeout, match="still running after 5 seconds"):
        wait_for_terminal(fake.fetch, 5, 1.0, fake.clock, fake.sleep)
