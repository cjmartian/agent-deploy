import { Link } from 'react-router-dom'
import ProjectCard from '../components/ProjectCard'
import { projects } from '../data/projects'

const featured = projects.filter(p => p.featured)

export default function Home() {
  return (
    <>
      <section className="hero">
        <div className="container">
          <div className="hero-content">
            <p className="hero-greeting animate-in">👽 Hi there, I'm</p>
            <h1 className="hero-name animate-in animate-delay-1">Connor Martin</h1>
            <p className="hero-title animate-in animate-delay-2">
              Senior Software Engineer @ <strong style={{ color: 'var(--color-text)' }}>GitHub</strong>
            </p>
            <p className="hero-desc animate-in animate-delay-3">
              Open-source enthusiast, former conda maintainer at Anaconda, and builder of developer tools.
              I love making connections with the community and working on things that make developers' lives easier.
            </p>
            <div className="hero-stats animate-in animate-delay-3">
              <div className="hero-stat">
                <div className="hero-stat-value">1,979</div>
                <div className="hero-stat-label">Contributions/yr</div>
              </div>
              <div className="hero-stat">
                <div className="hero-stat-value">42</div>
                <div className="hero-stat-label">Repositories</div>
              </div>
              <div className="hero-stat">
                <div className="hero-stat-value">100k+</div>
                <div className="hero-stat-label">Stars (projects)</div>
              </div>
            </div>
            <div className="hero-cta animate-in animate-delay-4">
              <Link to="/work" className="btn btn-primary">View My Work →</Link>
              <a href="https://github.com/cjmartian" target="_blank" rel="noopener noreferrer" className="btn btn-outline">
                GitHub Profile
              </a>
            </div>
          </div>
        </div>
      </section>

      <section className="featured-section">
        <div className="container">
          <h2 className="section-title">Featured Projects</h2>
          <p className="section-subtitle">
            Highlights from my open-source contributions across the Python and Go ecosystems.
          </p>
          <div className="card-grid">
            {featured.map(project => (
              <ProjectCard key={project.repo} {...project} />
            ))}
          </div>
        </div>
      </section>
    </>
  )
}
