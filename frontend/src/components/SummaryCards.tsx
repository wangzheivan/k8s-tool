interface StatCard {
  label: string;
  value: number | string;
  tone?: "ok" | "failed";
}

interface SummaryCardsProps {
  cards: StatCard[];
}

export function SummaryCards({ cards }: SummaryCardsProps) {
  return (
    <div className="stats">
      {cards.map((card) => (
        <div className="stat" key={card.label}>
          <div className="label">{card.label}</div>
          <div className={`value ${card.tone ?? ""}`}>{card.value}</div>
        </div>
      ))}
    </div>
  );
}
