import { Routes, Route } from 'react-router-dom'
import Layout from './components/Layout'
import Home from './pages/Home'
import Work from './pages/Work'
import Blog from './pages/Blog'
import BlogPost from './pages/BlogPost'
import Resume from './pages/Resume'
import Contact from './pages/Contact'
import Talks from './pages/Talks'
import OpenSource from './pages/OpenSource'

function App() {
  return (
    <Layout>
      <Routes>
        <Route path="/" element={<Home />} />
        <Route path="/work" element={<Work />} />
        <Route path="/blog" element={<Blog />} />
        <Route path="/blog/:slug" element={<BlogPost />} />
        <Route path="/resume" element={<Resume />} />
        <Route path="/contact" element={<Contact />} />
        <Route path="/talks" element={<Talks />} />
        <Route path="/opensource" element={<OpenSource />} />
      </Routes>
    </Layout>
  )
}

export default App
