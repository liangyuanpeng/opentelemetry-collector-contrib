// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package matcher // import "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/fileconsumer/matcher"

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/fileconsumer/matcher/internal/filter"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/fileconsumer/matcher/internal/finder"
)

const (
	sortTypeNumeric      = "numeric"
	sortTypeTimestamp    = "timestamp"
	sortTypeAlphabetical = "alphabetical"
)

const (
	defaultOrderingCriteriaTopN = 1
)

type Criteria struct {
	Include          []string         `mapstructure:"include,omitempty"`
	Exclude          []string         `mapstructure:"exclude,omitempty"`
	OrderingCriteria OrderingCriteria `mapstructure:"ordering_criteria,omitempty"`
}

type OrderingCriteria struct {
	Regex  string `mapstructure:"regex,omitempty"`
	TopN   int    `mapstructure:"top_n,omitempty"`
	SortBy []Sort `mapstructure:"sort_by,omitempty"`
}

type Sort struct {
	SortType  string `mapstructure:"sort_type,omitempty"`
	RegexKey  string `mapstructure:"regex_key,omitempty"`
	Ascending bool   `mapstructure:"ascending,omitempty"`

	// Timestamp only
	Layout   string `mapstructure:"layout,omitempty"`
	Location string `mapstructure:"location,omitempty"`
}

func New(c Criteria) (*Matcher, error) {
	if len(c.Include) == 0 {
		return nil, fmt.Errorf("'include' must be specified")
	}

	if err := finder.Validate(c.Include); err != nil {
		return nil, fmt.Errorf("include: %w", err)
	}
	if err := finder.Validate(c.Exclude); err != nil {
		return nil, fmt.Errorf("exclude: %w", err)
	}

	if len(c.OrderingCriteria.SortBy) == 0 {
		return &Matcher{
			include: c.Include,
			exclude: c.Exclude,
		}, nil
	}

	if c.OrderingCriteria.Regex == "" {
		return nil, fmt.Errorf("'regex' must be specified when 'sort_by' is specified")
	}

	if c.OrderingCriteria.TopN < 0 {
		return nil, fmt.Errorf("'top_n' must be a positive integer")
	}

	if c.OrderingCriteria.TopN == 0 {
		c.OrderingCriteria.TopN = defaultOrderingCriteriaTopN
	}

	regex, err := regexp.Compile(c.OrderingCriteria.Regex)
	if err != nil {
		return nil, fmt.Errorf("compile regex: %w", err)
	}

	var filterOpts []filter.Option
	for _, sc := range c.OrderingCriteria.SortBy {
		switch sc.SortType {
		case sortTypeNumeric:
			f, err := filter.SortNumeric(sc.RegexKey, sc.Ascending)
			if err != nil {
				return nil, fmt.Errorf("numeric sort: %w", err)
			}
			filterOpts = append(filterOpts, f)
		case sortTypeAlphabetical:
			f, err := filter.SortAlphabetical(sc.RegexKey, sc.Ascending)
			if err != nil {
				return nil, fmt.Errorf("alphabetical sort: %w", err)
			}
			filterOpts = append(filterOpts, f)
		case sortTypeTimestamp:
			f, err := filter.SortTemporal(sc.RegexKey, sc.Ascending, sc.Layout, sc.Location)
			if err != nil {
				return nil, fmt.Errorf("timestamp sort: %w", err)
			}
			filterOpts = append(filterOpts, f)
		default:
			return nil, fmt.Errorf("'sort_type' must be specified")
		}
	}

	return &Matcher{
		include:    c.Include,
		exclude:    c.Exclude,
		regex:      regex,
		topN:       c.OrderingCriteria.TopN,
		filterOpts: filterOpts,
	}, nil
}

type Matcher struct {
	include    []string
	exclude    []string
	regex      *regexp.Regexp
	topN       int
	filterOpts []filter.Option
}

// MatchFiles gets a list of paths given an array of glob patterns to include and exclude
func (m Matcher) MatchFiles() ([]string, error) {
	var errs error
	files, err := finder.FindFiles(m.include, m.exclude)
	if err != nil {
		errs = errors.Join(errs, err)
	}
	if len(files) == 0 {
		return files, errors.Join(fmt.Errorf("no files match the configured criteria"), errs)
	}
	if len(m.filterOpts) == 0 {
		return files, errs
	}

	result, err := filter.Filter(files, m.regex, m.filterOpts...)
	if len(result) == 0 {
		return result, errors.Join(err, errs)
	}

	if len(result) <= m.topN {
		return result, errors.Join(err, errs)
	}

	return result[:m.topN], errors.Join(err, errs)
}
