import ProjectCard from '../components/ProjectCard'
import { projects } from '../data/projects'

export default function Work() {
  return (
    <section className="section">
      <div className="container">
        <h1 className="section-title">Work & Projects</h1>
        <p className="section-subtitle">
          A collection of open-source projects I've contributed to, from package managers used by millions
          to developer tools and AI agent infrastructure.
        </p>
        <div className="card-grid">
          {projects.map(project => (
            <ProjectCard key={project.repo} {...project} />
          ))}
        </div>
      </div>
    </section>
  )
}
