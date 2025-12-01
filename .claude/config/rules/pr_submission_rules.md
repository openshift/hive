# PR Submission Rules for E2E Test Code

pr_submission_rules:
  template_requirements:
    - "Follow .github/pull_request_template.md from openshift-tests-private repository to create a PR"
  
  pr_creation:
    - "Create each PR with hold status"
  
  test_logs_location:
    - "Test logs can be found from: .claude/test_artifacts/{COMPONENT}/{JIRA_KEY}/test_execution_results/{JIRA_KEY}_test_execution_log.txt"
