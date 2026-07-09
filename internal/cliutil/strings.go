// Package cliutil holds shared helpers for the medical-device-intelligence CLI:
// query building, HTTP transport, and output rendering. English-only; every
// user-facing string in this project is inline English. There is no i18n layer
// and there must never be one.
package cliutil

// Disclaimer is printed on every human-facing result and embedded in every
// machine envelope. The CLI reports publicly available regulatory facts; it
// never states that a device is safe and never offers medical advice.
const Disclaimer = "Disclaimer: this tool reports public regulatory/records data (FDA, ClinicalTrials.gov, PubMed, and others). " +
	"It is not medical advice and is not a safety verdict. Absence of records is not evidence of safety. " +
	"Consult a regulator or a qualified clinician for medical decisions."

// NoRecordsMsg is the required phrasing for an empty result. We say "no records
// found" and never imply the subject is safe.
const NoRecordsMsg = "no records found"

// FDAClassLegend explains the FDA device classification shown in results.
const FDAClassLegend = "FDA device classes: " +
	"Class I - low risk (general controls); " +
	"Class II - moderate risk (special controls); " +
	"Class III - high risk (premarket approval)."
