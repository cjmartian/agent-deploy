import { Link } from 'react-router-dom'

interface ProjectCardProps {
  title: string
  repo: string
  description: string
  language: string
  stars?: string
  contributions?: string[]
  url: string
}

const langBadgeClass: Record<string, string> = {
  Python: 'badge-python',
  Go: 'badge-go',
  Ruby: 'badge-ruby',
  TypeScript: 'badge-typescript',
}

export default function ProjectCard({ title, repo, description, language, stars, contributions, url }: ProjectCardProps) {
  return (
    <a href={url} target="_blank" rel="noopener noreferrer" style={{ textDecoration: 'none', color: 'inherit' }}>
      <div className="card">
        <div className="card-title">{title}</div>
        <div style={{ fontFamily: 'var(--font-mono)', fontSize: '0.8rem', color: 'var(--color-text-dim)', marginBottom: '0.8rem' }}>
          {repo}
        </div>
        <div className="card-desc">{description}</div>
        {contributions && contributions.length > 0 && (
          <ul style={{ listStyle: 'none', padding: 0, marginBottom: '1rem' }}>
            {contributions.map((c, i) => (
              <li key={i} style={{ fontSize: '0.85rem', color: 'var(--color-text-muted)', padding: '0.2rem 0', paddingLeft: '1rem', position: 'relative' }}>
                <span style={{ position: 'absolute', left: 0, color: 'var(--color-accent-2)' }}>▸</span>
                {c}
              </li>
            ))}
          </ul>
        )}
        <div className="card-meta">
          <span className={`badge ${langBadgeClass[language] || ''}`}>{language}</span>
          {stars && <span className="badge badge-stars">★ {stars}</span>}
        </div>
      </div>
    </a>
  )
}

export function InternalProjectCard({ title, description, language, slug, stars }: {
  title: string; description: string; language: string; slug: string; stars?: string
}) {
  return (
    <Link to={slug} style={{ textDecoration: 'none', color: 'inherit' }}>
      <div className="card">
        <div className="card-title">{title}</div>
        <div className="card-desc">{description}</div>
        <div className="card-meta">
          <span className={`badge ${langBadgeClass[language] || ''}`}>{language}</span>
          {stars && <span className="badge badge-stars">★ {stars}</span>}
        </div>
      </div>
    </Link>
  )
}
