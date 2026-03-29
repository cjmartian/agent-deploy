import { useState, FormEvent } from 'react'

export default function Contact() {
  const [submitted, setSubmitted] = useState(false)

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    setSubmitted(true)
  }

  return (
    <section className="section">
      <div className="container">
        <h1 className="section-title">Get In Touch</h1>
        <p className="section-subtitle">
          Want to collaborate, chat about open source, or just say hi?
        </p>

        <div className="contact-grid">
          <div className="contact-links">
            <a href="https://github.com/cjmartian" target="_blank" rel="noopener noreferrer" className="contact-link">
              <div className="contact-link-icon">🐙</div>
              <div>
                <div style={{ fontWeight: 600 }}>GitHub</div>
                <div style={{ fontSize: '0.85rem', color: 'var(--color-text-muted)' }}>@cjmartian</div>
              </div>
            </a>
            <a href="https://www.linkedin.com/in/cjmartian/" target="_blank" rel="noopener noreferrer" className="contact-link">
              <div className="contact-link-icon">💼</div>
              <div>
                <div style={{ fontWeight: 600 }}>LinkedIn</div>
                <div style={{ fontSize: '0.85rem', color: 'var(--color-text-muted)' }}>/in/cjmartian</div>
              </div>
            </a>
            <div className="contact-link" style={{ cursor: 'default' }}>
              <div className="contact-link-icon">📍</div>
              <div>
                <div style={{ fontWeight: 600 }}>Location</div>
                <div style={{ fontSize: '0.85rem', color: 'var(--color-text-muted)' }}>Seattle, WA</div>
              </div>
            </div>
          </div>

          <div>
            {submitted ? (
              <div className="card" style={{ textAlign: 'center', padding: '3rem' }}>
                <div style={{ fontSize: '2rem', marginBottom: '1rem' }}>✨</div>
                <div style={{ fontSize: '1.2rem', fontWeight: 600, marginBottom: '0.5rem' }}>Thanks for reaching out!</div>
                <p style={{ color: 'var(--color-text-muted)' }}>This is a demo form. For now, connect via GitHub or LinkedIn.</p>
              </div>
            ) : (
              <form className="contact-form" onSubmit={handleSubmit}>
                <input type="text" placeholder="Your Name" required />
                <input type="email" placeholder="Your Email" required />
                <textarea placeholder="Your Message" required />
                <button type="submit" className="btn btn-primary" style={{ alignSelf: 'flex-start' }}>
                  Send Message →
                </button>
              </form>
            )}
          </div>
        </div>
      </div>
    </section>
  )
}
