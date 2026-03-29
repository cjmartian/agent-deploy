interface TimelineItemProps {
  date: string
  title: string
  company: string
  description: string[]
}

export default function TimelineItem({ date, title, company, description }: TimelineItemProps) {
  return (
    <div className="timeline-item">
      <div className="timeline-date">{date}</div>
      <div className="timeline-title">{title}</div>
      <div className="timeline-company">{company}</div>
      <div className="timeline-desc">
        <ul>
          {description.map((item, i) => (
            <li key={i}>{item}</li>
          ))}
        </ul>
      </div>
    </div>
  )
}
