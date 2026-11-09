"""The aggregation windows: seven civil days, and a refusal to accept anything else.

`precip_sum` is the feature where a plausible number is most expensive. `risk` sends
a seven-day accumulation in millimetres; a six-day sum, a daily mean or a rate would
all arrive under the same name and mean something else. These tests pin the window,
the unit of coverage and the rule that an incomplete window is absent instead of
smaller.
"""

from __future__ import annotations

import datetime as dt

import pytest

import assemble


def test_civil_day_window_is_ascending_and_inclusive():
    days = assemble.civil_day_window(dt.date(2019, 1, 7), 3)
    assert days == (dt.date(2019, 1, 5), dt.date(2019, 1, 6), dt.date(2019, 1, 7))


def test_civil_day_window_crosses_a_month_boundary_without_arithmetic():
    days = assemble.civil_day_window(dt.date(2019, 3, 2), 4)
    assert days[0] == dt.date(2019, 2, 27)
    assert days[-1] == dt.date(2019, 3, 2)


def test_seven_day_precipitation_is_the_accumulation_of_the_whole_window():
    end = dt.date(2019, 1, 7)
    days = assemble.civil_day_window(end, 7)
    readings = {day: [2.0] for day in days}
    readings[days[0]] = [0.5]  # the oldest day of the window is what a six-day sum would drop
    accumulation = assemble.precipitation_sum(readings, end=end)
    assert accumulation.value == pytest.approx(12.5)
    assert accumulation.covered_days == 7
    assert accumulation.required_days == 7


def test_six_day_window_is_refused_rather_than_computed():
    end = dt.date(2019, 1, 7)
    readings = {day: [2.0] for day in assemble.civil_day_window(end, 6)}
    with pytest.raises(assemble.WindowError) as refusal:
        assemble.precipitation_sum(readings, end=end, window_days=6)
    assert "7-civil-day" in str(refusal.value)


def test_incomplete_window_is_absent_not_a_partial_sum():
    end = dt.date(2019, 1, 7)
    days = assemble.civil_day_window(end, 7)
    readings = {day: [3.0] for day in days[:-1]}  # the earliest day has no reading
    accumulation = assemble.precipitation_sum(readings, end=end)
    assert accumulation.value is None
    assert accumulation.covered_days == 6
    assert "6 de 7" in accumulation.reason


def test_reduce_window_uses_the_artifact_reducers():
    end = dt.date(2019, 1, 7)
    readings = {end: [1.0, 3.0, 5.0]}
    assert assemble.reduce_window(readings, end=end, window_days=1, reducer="mean").value == 3.0
    assert assemble.reduce_window(readings, end=end, window_days=1, reducer="max").value == 5.0
    assert assemble.reduce_window(readings, end=end, window_days=1, reducer="min").value == 1.0
    assert assemble.reduce_window(readings, end=end, window_days=1, reducer="sum").value == 9.0


def test_unknown_reducer_is_refused():
    with pytest.raises(assemble.WindowError):
        assemble.reduce_window({}, end=dt.date(2019, 1, 7), window_days=1, reducer="median")
