import { useEffect, useState } from 'react'
import { useParams, Link } from 'react-router-dom'

interface BlogPostData {
  slug: string
  title: string
  date: string
  excerpt: string
  tags: string[]
  author: string
  content: string
}

export default function BlogPost() {
  const { slug } = useParams<{ slug: string }>()
  const [post, setPost] = useState<BlogPostData | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  useEffect(() => {
    if (!slug) return
    fetch(`/api/blog/${encodeURIComponent(slug)}`)
      .then(res => {
        if (!res.ok) throw new Error('not found')
        return res.json()
      })
      .then(data => {
        setPost(data)
        setLoading(false)
      })
      .catch(() => {
        setError(true)
        setLoading(false)
      })
  }, [slug])

  if (loading) {
    return (
      <section className="section">
        <div className="container">
          <div className="loading"><div className="loading-spinner" /></div>
        </div>
      </section>
    )
  }

  if (error || !post) {
    return (
      <section className="section">
        <div className="container">
          <Link to="/blog" className="back-link">← Back to Blog</Link>
          <h1 className="section-title">Post Not Found</h1>
          <p className="section-subtitle">This blog post doesn't exist or has been removed.</p>
        </div>
      </section>
    )
  }

  return (
    <section className="section">
      <div className="container">
        <div className="blog-content">
          <Link to="/blog" className="back-link">← Back to Blog</Link>
          <div className="blog-header">
            <h1 className="section-title">{post.title}</h1>
            <p style={{ fontFamily: 'var(--font-mono)', fontSize: '0.9rem', color: 'var(--color-accent-2)' }}>
              {new Date(post.date).toLocaleDateString('en-US', { year: 'numeric', month: 'long', day: 'numeric' })}
              {post.author && ` · ${post.author}`}
            </p>
            {post.tags && post.tags.length > 0 && (
              <div style={{ display: 'flex', gap: '0.5rem', marginTop: '1rem', flexWrap: 'wrap' }}>
                {post.tags.map(tag => (
                  <span key={tag} className="badge">{tag}</span>
                ))}
              </div>
            )}
          </div>
          <div className="blog-body" dangerouslySetInnerHTML={{ __html: post.content }} />
        </div>
      </div>
    </section>
  )
}
