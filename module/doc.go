// Package module is the Tier 1 module contract (PLAN §6.1): the Module interface and its
// optional ones, configs, actions, events and the registry modules add their kind to at init.
// Module errors reach logs and the UI, so they must not contain secrets.
package module
