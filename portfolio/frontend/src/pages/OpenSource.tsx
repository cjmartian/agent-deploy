import StatCard from '../components/StatCard'

const stats = [
  { value: '1,979', label: 'Contributions/Year' },
  { value: '42', label: 'Repositories' },
  { value: '69%', label: 'Commits' },
  { value: '16%', label: 'Code Review' },
  { value: '10%', label: 'Pull Requests' },
  { value: '5%', label: 'Issues' },
]

const achievements = [
  { icon: '🦈', name: 'Pull Shark', detail: 'x4 — Prolific PR author' },
  { icon: '👯', name: 'Pair Extraordinaire', detail: 'x3 — Co-authored commits' },
  { icon: '🏔️', name: 'Arctic Code Vault', detail: 'Contributor to preserved code' },
  { icon: '⚡', name: 'YOLO', detail: 'Merged without review' },
  { icon: '🎯', name: 'Quickdraw', detail: 'Rapid issue response' },
]

const topRepos = [
  { name: 'nvbn/thefuck', stars: '95.8k', lang: 'Python', role: 'Contributor' },
  { name: 'python/cpython', stars: '65k+', lang: 'Python', role: 'Contributor' },
  { name: 'mattermost/mattermost', stars: '30k+', lang: 'Go', role: 'Contributor' },
  { name: 'hashicorp/vagrant', stars: '26k+', lang: 'Ruby', role: 'Contributor' },
  { name: 'conda/conda', stars: '7.3k', lang: 'Python', role: 'Maintainer' },
  { name: 'PrefectHQ/prefect', stars: '17k+', lang: 'Python', role: 'Contributor' },
]

const langBreakdown = [
  { lang: 'Python', pct: 55, color: '#3776ab' },
  { lang: 'Go', pct: 25, color: '#00add8' },
  { lang: 'Ruby', pct: 8, color: '#cc342d' },
  { lang: 'Shell', pct: 7, color: '#89e051' },
  { lang: 'Other', pct: 5, color: '#8888a8' },
]

export default function OpenSource() {
  return (
    <section className="section">
      <div className="container">
        <h1 className="section-title">Open Source</h1>
        <p className="section-subtitle">
          A snapshot of my open-source activity and contributions across the GitHub ecosystem.
        </p>

        <div className="stat-grid">
          {stats.map(s => (
            <StatCard key={s.label} {...s} />
          ))}
        </div>

        <h2 style={{ fontSize: '1.8rem', fontWeight: 700, marginBottom: '1.5rem', color: 'var(--color-text)' }}>
          GitHub Achievements
        </h2>
        <div className="achievement-list">
          {achievements.map(a => (
            <div key={a.name} className="achievement">
              <span className="achievement-icon">{a.icon}</span>
              <div>
                <div className="achievement-name">{a.name}</div>
                <div className="achievement-detail">{a.detail}</div>
              </div>
            </div>
          ))}
        </div>

        <h2 style={{ fontSize: '1.8rem', fontWeight: 700, marginBottom: '1.5rem', marginTop: '2rem', color: 'var(--color-text)' }}>
          Language Breakdown
        </h2>
        <div style={{ marginBottom: '3rem' }}>
          <div style={{ display: 'flex', height: '12px', borderRadius: '6px', overflow: 'hidden', marginBottom: '1rem' }}>
            {langBreakdown.map(l => (
              <div key={l.lang} style={{ width: `${l.pct}%`, background: l.color, transition: 'width 0.5s' }} title={`${l.lang}: ${l.pct}%`} />
            ))}
          </div>
          <div style={{ display: 'flex', gap: '1.5rem', flexWrap: 'wrap' }}>
            {langBreakdown.map(l => (
              <div key={l.lang} style={{ display: 'flex', alignItems: 'center', gap: '0.4rem', fontSize: '0.85rem', color: 'var(--color-text-muted)' }}>
                <span style={{ width: '10px', height: '10px', borderRadius: '50%', background: l.color, display: 'inline-block' }} />
                {l.lang} ({l.pct}%)
              </div>
            ))}
          </div>
        </div>

        <h2 style={{ fontSize: '1.8rem', fontWeight: 700, marginBottom: '1.5rem', color: 'var(--color-text)' }}>
          Top Contributed Repos
        </h2>
        <div className="card-grid">
          {topRepos.map(r => (
            <a key={r.name} href={`https://github.com/${r.name}`} target="_blank" rel="noopener noreferrer" style={{ textDecoration: 'none', color: 'inherit' }}>
              <div className="card">
                <div className="card-title" style={{ fontSize: '1.1rem' }}>{r.name}</div>
                <div className="card-meta" style={{ marginTop: '0.8rem' }}>
                  <span className={`badge badge-${r.lang.toLowerCase()}`}>{r.lang}</span>
                  <span className="badge badge-stars">★ {r.stars}</span>
                  <span className="badge">{r.role}</span>
                </div>
              </div>
            </a>
          ))}
        </div>
      </div>
    </section>
  )
}
