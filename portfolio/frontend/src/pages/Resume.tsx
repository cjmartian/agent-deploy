import TimelineItem from '../components/TimelineItem'
import { career, skills } from '../data/career'

export default function Resume() {
  return (
    <section className="section">
      <div className="container">
        <h1 className="section-title">Resume</h1>
        <p className="section-subtitle">
          My career trajectory from package management to platform engineering.
        </p>

        <div className="timeline">
          {career.map((entry, i) => (
            <TimelineItem key={i} {...entry} />
          ))}
        </div>

        <div style={{ marginTop: '4rem' }}>
          <h2 style={{ fontSize: '1.8rem', fontWeight: 700, marginBottom: '2rem', color: 'var(--color-text)' }}>
            Skills & Technologies
          </h2>

          <div style={{ marginBottom: '2rem' }}>
            <h3 style={{ fontSize: '1rem', fontWeight: 600, color: 'var(--color-accent-2)', marginBottom: '0.8rem', textTransform: 'uppercase', letterSpacing: '0.1em' }}>
              Languages
            </h3>
            <div className="skills-grid">
              {skills.languages.map(s => <span key={s} className="skill-tag">{s}</span>)}
            </div>
          </div>

          <div style={{ marginBottom: '2rem' }}>
            <h3 style={{ fontSize: '1rem', fontWeight: 600, color: 'var(--color-accent-3)', marginBottom: '0.8rem', textTransform: 'uppercase', letterSpacing: '0.1em' }}>
              Tools & Infrastructure
            </h3>
            <div className="skills-grid">
              {skills.tools.map(s => <span key={s} className="skill-tag">{s}</span>)}
            </div>
          </div>

          <div style={{ marginBottom: '2rem' }}>
            <h3 style={{ fontSize: '1rem', fontWeight: 600, color: 'var(--color-accent-1)', marginBottom: '0.8rem', textTransform: 'uppercase', letterSpacing: '0.1em' }}>
              Domains
            </h3>
            <div className="skills-grid">
              {skills.domains.map(s => <span key={s} className="skill-tag">{s}</span>)}
            </div>
          </div>

          <div>
            <h3 style={{ fontSize: '1rem', fontWeight: 600, color: 'var(--color-accent-4)', marginBottom: '0.8rem', textTransform: 'uppercase', letterSpacing: '0.1em' }}>
              Platforms
            </h3>
            <div className="skills-grid">
              {skills.platforms.map(s => <span key={s} className="skill-tag">{s}</span>)}
            </div>
          </div>
        </div>
      </div>
    </section>
  )
}
