import type { ReactNode } from "react";

export function PageHeader({
  title,
  children,
}: {
  title: string;
  children?: ReactNode;
}) {
  return (
    <div className="border-border border-b px-6 pb-4 pt-2">
      <h1 className="font-heading text-[19px] font-bold tracking-tight">{title}</h1>
      <p className="text-muted-foreground mt-0.5 text-[12.5px]">{children}</p>
    </div>
  );
}

export function Mono({ children }: { children: ReactNode }) {
  return <span className="font-mono text-[0.95em]">{children}</span>;
}
