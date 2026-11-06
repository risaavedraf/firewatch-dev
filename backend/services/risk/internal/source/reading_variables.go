package source

import observations "firewatch.local/contracts/observations/v1"

// The readings history stores two families of variables: the five the shared
// observations contract already names, and the weather variables the trained
// artifact needs that the shared contract does not name yet.
//
// They are declared HERE, in the service that stores them, instead of in
// contracts/gen/observations/v1, because extending a versioned contract every
// service consumes is a change to the contract itself and this is a change to
// one service. The names are the canonical ones the classifier's factor table
// already uses (direccion_viento, precipitacion, radiacion_solar), so a reader
// can join a persisted factor to the history row it was computed from. When the
// shared vocabulary grows these constants become aliases for it and nothing else
// moves.
const (
	// VariableWindDirection is the wind direction in degrees, meteorological
	// convention: the direction the wind blows FROM. 0 and 360 are both north, so
	// a value is sent as measured rather than normalized here.
	VariableWindDirection = "direccion_viento"
	// VariablePrecipitation is one precipitation AMOUNT in millimetres, not a
	// rate. The readings are summed, so a source that publishes mm/h under this
	// name would be summed into a meaningless total; the unit check cannot
	// distinguish the two, which is why the amount semantics is stated here.
	VariablePrecipitation = "precipitacion"
	// VariableRadiation is one instantaneous solar radiation reading in W/m2,
	// matching what rtdata publishes. It is deliberately not converted into a
	// daily integral: if the training contract ever needs one, the artifact's
	// declared unit changes with it.
	VariableRadiation = "radiacion_solar"
)

// WeatherVariableNames returns the variables this service stores on top of the
// shared canonical vocabulary, in a stable order.
func WeatherVariableNames() []string {
	return []string{VariableWindDirection, VariablePrecipitation, VariableRadiation}
}

// ReadingVariableNames returns every variable the readings history stores, in a
// stable order: the shared canonical vocabulary first, then the weather
// variables this service adds. It is a copy, so no caller can mutate it.
func ReadingVariableNames() []string {
	return append(observations.VariableNames(), WeatherVariableNames()...)
}

// expectedUnits is the one unit each stored variable is expressed in.
//
// The comparison is exact and there are no aliases on purpose. A reading that
// arrives in another unit - `°F` for a temperature stored in `C`, `m/s` for a
// wind reading stored in `km/h` - is not converted here and must not enter an
// aggregate: converting silently is a reinterpretation of a source's value, and
// rejecting loudly is what makes the source fix its unit. A unit that is merely
// spelled differently is still a different unit as far as this table is
// concerned; adding an alias is a deliberate edit with its own justification.
var expectedUnits = map[string]string{
	observations.VariableTemperature:     "C",
	observations.VariableHumidity:        "%",
	observations.VariableWind:            "km/h",
	observations.VariableVegetation:      "%",
	observations.VariableHistoricalFires: "count",
	VariableWindDirection:                "deg",
	VariablePrecipitation:                "mm",
	VariableRadiation:                    "W/m2",
}

// ExpectedUnit returns the unit a variable's reading must be expressed in, and
// whether that variable has a declared unit at all. A variable with no declared
// unit is not unit-checked, so extending the vocabulary does not retroactively
// reject readings of a variable nobody has assigned a unit yet.
func ExpectedUnit(variable string) (string, bool) {
	unit, declared := expectedUnits[variable]
	return unit, declared
}

// UnitMatches reports whether a reading's unit is the one its variable is stored
// in. An undeclared variable matches no unit, which is why callers that must not
// guess ask ExpectedUnit first.
func UnitMatches(variable, unit string) bool {
	expected, declared := expectedUnits[variable]
	return declared && unit == expected
}
