import { useEffect } from 'react'
import { Link } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { MessageSquare, ShieldCheck, LayoutDashboard, ArrowRight } from 'lucide-react'

const FEATURES = [
  {
    icon: MessageSquare,
    title: 'Calificare prin AI',
    desc: 'Asistentul nostru colectează cererea ta printr-o conversație naturală în română, fără formulare complicate.',
  },
  {
    icon: ShieldCheck,
    title: 'Verificare ANAF',
    desc: 'CUI-ul companiei tale este verificat automat în baza de date ANAF pentru o calificare rapidă și corectă.',
  },
  {
    icon: LayoutDashboard,
    title: 'Dashboard Centralizat',
    desc: 'Toate cererile și conversațiile într-un singur loc, accesibile oricând de echipa ta comercială.',
  },
]

export function LandingPage() {
  useEffect(() => {
    const apiUrl = import.meta.env.VITE_API_URL ?? ''
    import('../../widget/widget-entry').then(() => {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ;(window as any).AngrosistChat?.init({ apiUrl })
    })
  }, [])

  return (
    <div className="flex flex-col">
      {/* ── Hero ─────────────────────────────────────────────── */}
      <section className="flex flex-col items-center justify-center text-center px-4 pt-20 pb-24 md:pt-32 md:pb-36">
        <span className="inline-flex items-center rounded-full border px-3 py-1 text-xs font-medium text-muted-foreground mb-6">
          Platformă B2B En-Gros · România
        </span>

        <h1 className="text-4xl sm:text-5xl md:text-6xl font-bold tracking-tight leading-tight max-w-3xl">
          Achiziții en-gros,
          <br />
          <span className="text-primary">simplificate</span>
        </h1>

        <p className="mt-6 text-base sm:text-lg text-muted-foreground max-w-xl">
          Conectăm cumpărătorii B2B cu furnizorii potriviți printr-un asistent AI disponibil 24/7 — fără apeluri, fără așteptare.
        </p>

        <div className="mt-10 flex flex-col sm:flex-row items-center gap-3 w-full sm:w-auto">
          <Button size="lg" className="w-full sm:w-auto gap-2" asChild>
            <Link to="/chat">
              Pornește o conversație
              <ArrowRight className="h-4 w-4" />
            </Link>
          </Button>
          <Button size="lg" variant="outline" className="w-full sm:w-auto" asChild>
            <Link to="/dashboard">
              Vezi dashboard
            </Link>
          </Button>
        </div>
      </section>

      {/* ── Features ─────────────────────────────────────────── */}
      <section className="border-t bg-muted/40">
        <div className="max-w-5xl mx-auto px-4 py-16 md:py-24">
          <h2 className="text-center text-2xl sm:text-3xl font-semibold mb-12">
            Cum funcționează
          </h2>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-8">
            {FEATURES.map(({ icon: Icon, title, desc }) => (
              <div key={title} className="flex flex-col gap-3">
                <div className="flex items-center justify-center h-11 w-11 rounded-xl bg-primary/10">
                  <Icon className="h-5 w-5 text-primary" />
                </div>
                <h3 className="font-semibold text-base">{title}</h3>
                <p className="text-sm text-muted-foreground leading-relaxed">{desc}</p>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* ── CTA strip ────────────────────────────────────────── */}
      <section className="border-t px-4 py-16 text-center">
        <h2 className="text-2xl sm:text-3xl font-semibold mb-3">Gata să începi?</h2>
        <p className="text-muted-foreground mb-8 max-w-sm mx-auto text-sm">
          Folosește butonul de chat din colțul din dreapta jos sau deschide pagina de chat dedicată.
        </p>
        <Button size="lg" asChild>
          <Link to="/chat">Deschide chat complet</Link>
        </Button>
      </section>
    </div>
  )
}
