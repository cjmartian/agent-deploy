export interface Project {
  title: string
  repo: string
  description: string
  language: string
  stars?: string
  contributions?: string[]
  url: string
  featured?: boolean
}

export const projects: Project[] = [
  {
    title: 'conda',
    repo: 'conda/conda',
    description: 'A system-level, binary package and environment manager running on all major operating systems and platforms.',
    language: 'Python',
    stars: '7.3k',
    contributions: [
      'Core maintainer and code reviewer',
      'Fixed URL parsing and environment channel handling',
      'Improved CLI help output and argparse behavior',
      'Colorama compatibility fixes across platforms',
    ],
    url: 'https://github.com/conda/conda',
    featured: true,
  },
  {
    title: 'thefuck',
    repo: 'nvbn/thefuck',
    description: 'Magnificent app which corrects your previous console command.',
    language: 'Python',
    stars: '95.8k',
    contributions: [
      'Added conda rule for command correction (#1138)',
      'Implemented git-lfs support (#1056)',
      'Docker: remove container before image fix (#928)',
    ],
    url: 'https://github.com/nvbn/thefuck',
    featured: true,
  },
  {
    title: 'CPython',
    repo: 'python/cpython',
    description: 'The Python programming language — the reference implementation.',
    language: 'Python',
    stars: '65k+',
    contributions: [
      'Arctic Code Vault Contributor',
      'Contributions to the core Python runtime',
    ],
    url: 'https://github.com/python/cpython',
    featured: true,
  },
  {
    title: 'Mattermost Server',
    repo: 'mattermost/mattermost-server',
    description: 'Open source Slack-alternative in Golang and React.',
    language: 'Go',
    stars: '30k+',
    contributions: [
      'Worked on server core and plugin ecosystem',
      'Developed GitHub, MS Calendar, and MS Teams plugins',
    ],
    url: 'https://github.com/mattermost/mattermost',
  },
  {
    title: 'Mattermost GitHub Plugin',
    repo: 'mattermost/mattermost-plugin-github',
    description: 'GitHub integration plugin for the Mattermost platform.',
    language: 'Go',
    contributions: [
      'Built GitHub-to-Mattermost integration features',
    ],
    url: 'https://github.com/mattermost/mattermost-plugin-github',
  },
  {
    title: 'MCP Go Starter',
    repo: 'cjmartian/mcp-go-starter',
    description: 'A starter repo for building a Go MCP (Model Context Protocol) server.',
    language: 'Go',
    contributions: [
      'Building tools for AI agent infrastructure',
    ],
    url: 'https://github.com/cjmartian/mcp-go-starter',
  },
  {
    title: 'MCP Python Starter',
    repo: 'cjmartian/mcp-python-starter',
    description: 'A starter MCP server in Python for AI agent tooling.',
    language: 'Python',
    contributions: [
      'MCP server template for the Python ecosystem',
    ],
    url: 'https://github.com/cjmartian/mcp-python-starter',
  },
  {
    title: 'octocatalog-diff',
    repo: 'github/octocatalog-diff',
    description: 'Compile Puppet catalogs from 2 branches, versions, etc., and compare them.',
    language: 'Ruby',
    contributions: [
      'Infrastructure-as-code tooling at GitHub',
    ],
    url: 'https://github.com/github/octocatalog-diff',
  },
  {
    title: 'pytest-split',
    repo: 'jerry-git/pytest-split',
    description: 'Pytest plugin which splits the test suite to equally sized sub suites based on test execution time.',
    language: 'Python',
    contributions: [
      'CI/CD optimization for test parallelization',
    ],
    url: 'https://github.com/jerry-git/pytest-split',
  },
  {
    title: 'conda-forge Feedstocks',
    repo: 'conda-forge/*',
    description: 'Community-driven collection of conda package recipes. Maintained multiple feedstocks.',
    language: 'Python',
    contributions: [
      'Maintained boto3, botocore, beautifulsoup4, python-magic, arrow-cpp feedstocks',
      'Managed conda-build feedstock and CI pipelines',
    ],
    url: 'https://github.com/conda-forge',
  },
  {
    title: 'anaconda-linter',
    repo: 'cjmartian/anaconda-linter',
    description: 'Linting tool for Anaconda conda recipes and package configurations.',
    language: 'Python',
    contributions: [
      'Built linting infrastructure for conda package quality',
    ],
    url: 'https://github.com/cjmartian/anaconda-linter',
  },
  {
    title: 'Vagrant',
    repo: 'hashicorp/vagrant',
    description: 'A tool for building and distributing development environments.',
    language: 'Ruby',
    stars: '26k+',
    contributions: [
      'Contributions to the HashiCorp ecosystem',
    ],
    url: 'https://github.com/hashicorp/vagrant',
  },
]
