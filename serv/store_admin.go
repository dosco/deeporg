package serv

// DeepORG creates and migrates its own store when it starts. There is no
// separate step to run first, and no setting that turns that off: a store it
// cannot create its tables in is a store it cannot use.

// storeTableNames lists the unqualified control-plane tables the current
// config expects.
func (s *graphjinService) storeTableNames() []string {
	if s == nil {
		return nil
	}
	return configStoreTableNames(s.conf)
}

// configStoreTableNames lists the store tables a config creates. Schema
// discovery depends on it, so it is part of the discovery fingerprint: a
// table switched on by agent config (approvals) must not reuse a generation
// discovered before the table existed.
func configStoreTableNames(conf *Config) []string {
	if conf == nil || !conf.Core.Artifacts.Enabled {
		return nil
	}
	names := []string{"artifacts", "revisions"}
	if configWatchesEnabled(conf) {
		names = append(names, "watches", "watch_events")
	}
	if configTasksEnabled(conf) {
		names = append(names, "tasks", "task_entries")
	}
	if configThreadsEnabled(conf) {
		names = append(names, "threads", "messages", "runs", "run_events")
	}
	if configApprovalsEnabled(conf) {
		names = append(names, "approvals")
	}
	if configNotificationsEnabled(conf) {
		names = append(names, "notifications")
	}
	if configBudgetsEnabled(conf) {
		names = append(names, "agent_usage")
	}
	return names
}
