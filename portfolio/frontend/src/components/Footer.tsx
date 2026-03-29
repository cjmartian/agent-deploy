import { Link } from 'react-router-dom'

export default function Footer() {
  return (
    <footer className="footer">
      <div className="footer-inner">
        <ul className="footer-links">
          <li><Link to="/">Home</Link></li>
          <li><Link to="/work">Work</Link></li>
          <li><Link to="/blog">Blog</Link></li>
          <li><a href="https://github.com/cjmartian" target="_blank" rel="noopener noreferrer">GitHub</a></li>
          <li><a href="https://www.linkedin.com/in/cjmartian/" target="_blank" rel="noopener noreferrer">LinkedIn</a></li>
        </ul>
        <p className="footer-copy">© {new Date().getFullYear()} Connor Martin. Built with Go + React.</p>
      </div>
    </footer>
  )
}
