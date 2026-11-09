# Meteorology fixture provenance

Four recorded responses, two sources, two coordinates, one seven-day window that
crosses the New Year. They are the bytes the services actually returned, stored
verbatim: the client parses them exactly as it parses a live response, and
`tests/test_meteo.py` recomputes each sha256 and checks it appears in this file.

| | |
| --- | --- |
| Window | `2015-12-28 .. 2016-01-03` (7 civil days, crossing 31 December -> 1 January) |
| Retrieval date | 2026-11-06 (UTC) |
| Coordinates | the committed centroids of CUT `05101` (Valparaíso, -33.129605 / -71.572904) and CUT `05601` (San Antonio, -33.664116 / -71.489323), from `backend/training/data/comunas-valparaiso-centroids.json` |
| Licence | see per-source rows below |
| Recorded by | `backend/training/meteo.py --source <name> --cut <cut> --start 2015-12-28 --end 2016-01-03`, then copied out of the cache byte for byte |
| Committed bytes | unchanged. Tests that need an impossible response build it in memory from one of these files and say so; no committed fixture is edited by a test |

## Open-Meteo

| | |
| --- | --- |
| Endpoint | `https://archive-api.open-meteo.com/v1/archive` |
| Licence | free for non-commercial use; no API key, no registration |
| Attribution | Open-Meteo (<https://open-meteo.com>), ERA5 reanalysis archive |
| Day boundary | civil day in `America/Santiago` (the request names it, the response echoes it) |
| Parameters | `temperature_2m_max`, `temperature_2m_min`, `relative_humidity_2m_mean`, `wind_speed_10m_max`, `precipitation_sum`, `shortwave_radiation_sum` |
| Declared units | `°C`, `°C`, `%`, `km/h`, `mm`, `MJ/m²` |

| File | Bytes | sha256 |
| --- | --- | --- |
| `open-meteo-05101-2015-12-28_2016-01-03.json` | 860 | `f6390886bc7cdd24ffd67dd4676e2f879108a16bf85e4c74ff6505969001482d` |
| `open-meteo-05601-2015-12-28_2016-01-03.json` | 861 | `5bf32895b0e3706070406489a0796cc237d1a8d991c36bef8d9e1aeb1f1ad37b` |

Exact URL of the first file:

```text
https://archive-api.open-meteo.com/v1/archive?latitude=-33.129605&longitude=-71.572904&start_date=2015-12-28&end_date=2016-01-03&daily=temperature_2m_max,temperature_2m_min,relative_humidity_2m_mean,wind_speed_10m_max,precipitation_sum,shortwave_radiation_sum&timezone=America%2FSantiago
```

### `utc_offset_seconds` is recorded, not used

The response declares `"timezone": "America/Santiago"` **and** a single
`utc_offset_seconds`. Only the first is used to identify the civil day, because the
second is one number for the whole range and cannot describe a range that crosses a
DST change. Observed while recording: a July 2023 range came back with
`utc_offset_seconds = -10800` where tzdata says `-04:00` for those dates. That is why
the client takes the days from the echoed calendar and checks it, and why
`utc_offset_seconds == 0` is a refusal (0 means the day boundary was UTC, which
`America/Santiago` never is).

## NASA POWER

| | |
| --- | --- |
| Endpoint | `https://power.larc.nasa.gov/api/temporal/daily/point` |
| Licence | no credential and no non-commercial restriction; POWER asks to be cited and asks users to read its data disclaimer |
| Attribution | NASA Langley Research Center (LaRC) Prediction Of Worldwide Energy Resources (POWER) project |
| Day boundary | `LST` (the response's `header.time_standard`): a local standard-time day with **no DST**, so it is not guaranteed to be an `America/Santiago` civil day |
| Grid | 0.5 x 0.625 degrees, coarser than Open-Meteo's; the response's `geometry` echoes the cell it answered for |
| Parameters | `T2M_MAX`, `T2M_MIN`, `RH2M`, `WS2M_MAX`, `PRECTOTCORR`, `ALLSKY_SFC_SW_DWN` with `community=AG` |
| Declared units | `C`, `C`, `%`, `m/s`, `mm/day`, `MJ/m^2/day` |

| File | Bytes | sha256 |
| --- | --- | --- |
| `nasa-power-05101-2015-12-28_2016-01-03.json` | 1639 | `d61ba42934d428462f76f1618757fed4f680f4aa1857eab1bbca067e70b6fc1f` |
| `nasa-power-05601-2015-12-28_2016-01-03.json` | 1641 | `153e588bc03caf73c8d7d7567b4631af86807d6375703595701922a72e74ab03` |

Exact URL of the first file:

```text
https://power.larc.nasa.gov/api/temporal/daily/point?parameters=T2M_MAX,T2M_MIN,RH2M,WS2M_MAX,PRECTOTCORR,ALLSKY_SFC_SW_DWN&community=AG&longitude=-71.572904&latitude=-33.129605&start=20151228&end=20160103&format=JSON
```

`mm/day` and `MJ/m^2/day` are the daily accumulations the daily endpoint reports, so
summing N days gives `mm` and `MJ/m^2`. Missing values arrive as
`header.fill_value` (`-999.0`) and are read as "no reading", never as a measurement.

## What the window is chosen to cover

| Case | Where |
| --- | --- |
| A daily series that crosses 31 December -> 1 January | every fixture: `20151230`, `20151231`, `20160101` are all present |
| Real precipitation, including dry days | both sources: 0.0 mm days exist next to wet ones, which is how a measured zero differs from an absent day |
| `wind_speed_10m_max` in km/h that must become m/s | Open-Meteo fixtures |
| Two different parameter unit styles (`mm` vs `mm/day`) | both sources |
| A second coordinate, so a centroid bug cannot hide | both sources |

## What no honest recording contains

The refusal paths are driven from these files **in memory**, never by editing them:
a dropped day, a `null` value, a POWER fill value on one day, a changed declared
unit, a UTC calendar echo. Each mutation is one line of a test and the fixture on
disk stays the bytes the service returned.
