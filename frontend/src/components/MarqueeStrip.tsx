import React from "react";

export function FrameworkMarquee() {
  const frameworks = [
    "Next.js 14/15",
    "FastAPI (Python 3.12)",
    "Vite / React / Vue",
    "Django & Celery",
    "NestJS & Express",
    "Astro & SvelteKit",
    "Flask & Generic Node"
  ];

  return (
    <div className="marquee-container" aria-label="Supported Frameworks ticker">
      <div className="marquee-track">
        {frameworks.map((fw, i) => (
          <React.Fragment key={i}>
            <div className="marquee-item">
              <span className="marquee-badge">{fw}</span>
            </div>
            <span className="marquee-separator">✦</span>
          </React.Fragment>
        ))}
      </div>
      <div className="marquee-track" aria-hidden="true">
        {frameworks.map((fw, i) => (
          <React.Fragment key={`dup-${i}`}>
            <div className="marquee-item">
              <span className="marquee-badge">{fw}</span>
            </div>
            <span className="marquee-separator">✦</span>
          </React.Fragment>
        ))}
      </div>
    </div>
  );
}

export function InfraMarquee() {
  const services = [
    "🐘 Postgres 16 Alpine (:5432)",
    "⚡ Redis 7 Alpine (:6379)",
    "🍃 MongoDB 7 (:27017)",
    "🐬 MySQL 8 (:3306)",
    "🐇 RabbitMQ 3 (:5672)",
    "🔍 Elasticsearch 8 (:9200)"
  ];

  return (
    <div className="marquee-container marquee-container-alt" aria-label="Inferred Infrastructure ticker">
      <div className="marquee-track">
        {services.map((srv, i) => (
          <React.Fragment key={i}>
            <div className="marquee-item">
              <span className="marquee-badge">{srv}</span>
            </div>
            <span className="marquee-separator">✦</span>
          </React.Fragment>
        ))}
      </div>
      <div className="marquee-track" aria-hidden="true">
        {services.map((srv, i) => (
          <React.Fragment key={`dup-${i}`}>
            <div className="marquee-item">
              <span className="marquee-badge">{srv}</span>
            </div>
            <span className="marquee-separator">✦</span>
          </React.Fragment>
        ))}
      </div>
    </div>
  );
}
