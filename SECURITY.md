# Security policy

Report vulnerabilities privately using this repository's **Security → Report a vulnerability** option. Please do not include vulnerabilities or API keys in public issues.

Reports involving Silo or Sonarr credentials, request destinations, administrator access, status output, and release integrity are in scope. This plugin reads active Silo sessions and can change Sonarr monitoring and request episode searches. API keys are supplied through Silo's secret configuration fields and sent in request headers.

Security fixes are released for the latest published version. Upgrade to the latest release when reporting an issue and include the Silo version, plugin version, and reproduction steps with secrets removed.

Maintainer: [@CManson38](https://github.com/CManson38).
