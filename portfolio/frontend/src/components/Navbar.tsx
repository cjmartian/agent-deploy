import { useState } from 'react'
import { Link, useLocation } from 'react-router-dom'

const links = [
  { path: '/', label: 'Home' },
  { path: '/work', label: 'Work' },
  { path: '/opensource', label: 'Open Source' },
  { path: '/resume', label: 'Resume' },
  { path: '/blog', label: 'Blog' },
  { path: '/talks', label: 'Talks' },
  { path: '/contact', label: 'Contact' },
]

export default function Navbar() {
  const [open, setOpen] = useState(false)
  const location = useLocation()

  return (
    <nav className="navbar">
      <div className="navbar-inner">
        <Link to="/" className="navbar-logo">CM</Link>
        <button className="navbar-toggle" onClick={() => setOpen(!open)} aria-label="Toggle navigation">
          {open ? '✕' : '☰'}
        </button>
        <ul className={`navbar-links ${open ? 'open' : ''}`}>
          {links.map(({ path, label }) => (
            <li key={path}>
              <Link
                to={path}
                className={location.pathname === path ? 'active' : ''}
                onClick={() => setOpen(false)}
              >
                {label}
              </Link>
            </li>
          ))}
        </ul>
      </div>
    </nav>
  )
}
