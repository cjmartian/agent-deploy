import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'

interface BlogPostMeta {
  slug: string
  title: string
  date: string
  excerpt: string
  tags: string[]
  author: string
}

export default function Blog() {
  const [posts, setPosts] = useState<BlogPostMeta[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    fetch('/api/blog')
      .then(res => res.json())
      .then(data => {
        setPosts(data || [])
        setLoading(false)
      })
      .catch(() => setLoading(false))
  }, [])

  if (loading) {
    return (
      <section className="section">
        <div className="container">
          <div className="loading"><div className="loading-spinner" /></div>
        </div>
      </section>
    )
  }

  return (
    <section className="section">
      <div className="container">
        <h1 className="section-title">Blog</h1>
        <p className="section-subtitle">
          Thoughts on open source, developer tools, and building software.
        </p>
        {posts.length === 0 ? (
          <div className="talks-empty">
            <div className="talks-empty-icon">📝</div>
            <p className="talks-empty-text">Posts coming soon. Stay tuned!</p>
          </div>
        ) : (
          <div className="card-grid">
            {posts.map(post => (
              <Link key={post.slug} to={`/blog/${post.slug}`} style={{ textDecoration: 'none', color: 'inherit' }}>
                <div className="card blog-card">
                  <div className="card-date">{new Date(post.date).toLocaleDateString('en-US', { year: 'numeric', month: 'long', day: 'numeric' })}</div>
                  <div className="card-title">{post.title}</div>
                  <div className="card-desc">{post.excerpt}</div>
                  {post.tags && post.tags.length > 0 && (
                    <div className="card-tags">
                      {post.tags.map(tag => (
                        <span key={tag} className="badge">{tag}</span>
                      ))}
                    </div>
                  )}
                </div>
              </Link>
            ))}
          </div>
        )}
      </div>
    </section>
  )
}
