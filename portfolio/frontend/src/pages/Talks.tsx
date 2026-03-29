import { talks } from '../data/talks'

export default function Talks() {
  return (
    <section className="section">
      <div className="container">
        <h1 className="section-title">Speaking & Talks</h1>
        <p className="section-subtitle">
          Presentations and talks on open source, developer tools, and software engineering.
        </p>

        {talks.length === 0 ? (
          <div className="talks-empty">
            <div className="talks-empty-icon">🎤</div>
            <p className="talks-empty-text">
              Speaking engagements coming soon.<br />
              In the meantime, check out my <a href="https://github.com/cjmartian" target="_blank" rel="noopener noreferrer">open-source work</a>.
            </p>
          </div>
        ) : (
          <div className="card-grid">
            {talks.map((talk, i) => (
              <div key={i} className="card">
                <div className="card-date" style={{ fontFamily: 'var(--font-mono)', fontSize: '0.8rem', color: 'var(--color-accent-2)', marginBottom: '0.5rem' }}>
                  {talk.date} · {talk.event}
                </div>
                <div className="card-title">{talk.title}</div>
                <div className="card-desc">{talk.description}</div>
                {talk.url && (
                  <a href={talk.url} target="_blank" rel="noopener noreferrer" className="btn btn-outline" style={{ marginTop: '1rem', display: 'inline-block' }}>
                    Watch →
                  </a>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </section>
  )
}
