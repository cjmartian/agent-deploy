export interface CareerEntry {
  date: string
  title: string
  company: string
  description: string[]
}

export const career: CareerEntry[] = [
  {
    date: '2022 — Present',
    title: 'Senior Software Engineer',
    company: 'GitHub',
    description: [
      'Building developer tools and infrastructure at scale',
      'Working on internal tooling including Puppet catalog diffing (octocatalog-diff)',
      'Contributing to the MCP (Model Context Protocol) ecosystem for AI agents',
      'Active in open-source community with 1,900+ annual contributions',
      'Pull Shark x4 — consistent, high-quality pull request contributions',
    ],
  },
  {
    date: '2019 — 2022',
    title: 'Software Engineer',
    company: 'Anaconda',
    description: [
      'Core maintainer of conda — the package manager with 7.3k+ GitHub stars',
      'Worked across the entire Python distribution ecosystem',
      'Maintained conda-forge feedstocks: boto3, botocore, beautifulsoup4, arrow-cpp, and more',
      'Built anaconda-linter for conda recipe quality assurance',
      'Managed CI/CD pipelines with conda-concourse-ci',
      'Contributed to conda governance and community processes',
      'Collaborated on conda-build, conda-smithy, and recipe tooling',
    ],
  },
  {
    date: '2019',
    title: 'Open Source Contributor',
    company: 'Mattermost',
    description: [
      'Contributed to Mattermost server (Go) — open-source Slack alternative',
      'Developed plugins: GitHub integration, MS Calendar, MS Teams Meetings',
      'Full-stack work across Go backend and React frontend',
    ],
  },
]

export const skills = {
  languages: ['Python', 'Go', 'Ruby', 'TypeScript', 'JavaScript', 'Shell/Bash', 'C++'],
  tools: ['Docker', 'Kubernetes', 'AWS', 'GitHub Actions', 'Puppet', 'Ansible', 'Concourse CI'],
  domains: ['Package Management', 'CI/CD', 'Developer Tools', 'Infrastructure', 'Open Source', 'MCP/AI Agents'],
  platforms: ['conda', 'conda-forge', 'PyPI', 'GitHub', 'Mattermost'],
}
