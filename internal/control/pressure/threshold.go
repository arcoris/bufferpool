package pressure

import "errors"

const (
	// errThresholdMediumAboveHigh explains contradictory medium/high ordering.
	errThresholdMediumAboveHigh = "control/pressure: medium threshold must be <= high threshold"

	// errThresholdHighAboveCritical explains contradictory high/critical ordering.
	errThresholdHighAboveCritical = "control/pressure: high threshold must be <= critical threshold"

	// errThresholdMediumAboveCritical explains contradictory medium/critical ordering.
	errThresholdMediumAboveCritical = "control/pressure: medium threshold must be <= critical threshold"
)

// Thresholds contains optional level thresholds.
type Thresholds struct {
	// Medium is the optional medium pressure threshold.
	Medium uint64

	// High is the optional high pressure threshold.
	High uint64

	// Critical is the optional critical pressure threshold.
	Critical uint64
}

// Classifier is a prepared threshold evaluator for stable pressure policy.
//
// Controller loops usually classify many observations against the same
// thresholds. Preparing the classifier validates ordering once and keeps the
// repeated classification path explicit and allocation-free.
type Classifier struct {
	thresholds Thresholds
}

// NewClassifier returns a prepared classifier for thresholds.
func NewClassifier(thresholds Thresholds) (Classifier, error) {
	if err := thresholds.Validate(); err != nil {
		return Classifier{}, err
	}
	return Classifier{thresholds: thresholds}, nil
}

// MustNewClassifier returns a prepared classifier or panics for invalid thresholds.
//
// It is intended for tests and static defaults where invalid threshold ordering
// is a programming error.
func MustNewClassifier(thresholds Thresholds) Classifier {
	classifier, err := NewClassifier(thresholds)
	if err != nil {
		panic(err)
	}
	return classifier
}

// Classify returns the highest configured pressure level exceeded by value.
func (c Classifier) Classify(value uint64) Level {
	return Classify(value, c.thresholds)
}

// Classify returns the highest configured pressure level exceeded by value.
func Classify(value uint64, thresholds Thresholds) Level {
	if thresholds.Critical != 0 && value >= thresholds.Critical {
		return LevelCritical
	}
	if thresholds.High != 0 && value >= thresholds.High {
		return LevelHigh
	}
	if thresholds.Medium != 0 && value >= thresholds.Medium {
		return LevelMedium
	}
	return LevelNormal
}

// Validate checks configured thresholds for non-contradictory ordering.
func (t Thresholds) Validate() error {
	if t.Medium != 0 && t.High != 0 && t.Medium > t.High {
		return errors.New(errThresholdMediumAboveHigh)
	}
	if t.High != 0 && t.Critical != 0 && t.High > t.Critical {
		return errors.New(errThresholdHighAboveCritical)
	}
	if t.Medium != 0 && t.Critical != 0 && t.Medium > t.Critical {
		return errors.New(errThresholdMediumAboveCritical)
	}
	return nil
}
