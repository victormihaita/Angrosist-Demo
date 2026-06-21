/**
 * Structured user-facing strings for the dashboard (RO primary). Centralized so
 * a second language can be added without touching components — one language per
 * session, matching the agent's language stickiness. RO is the only active
 * locale today; the shape is ready for an `en` sibling.
 */
export const t = {
  common: {
    loading: 'Se încarcă...',
    retry: 'Reîncearcă',
    save: 'Salvează',
    saving: 'Se salvează...',
    cancel: 'Anulează',
    back: 'Înapoi',
    none: '—',
    all: 'Toate',
    error: 'A apărut o eroare.',
  },
  auth: {
    loginTitle: 'Autentificare',
    loginSubtitle: 'Accesează panoul de consultant Euro Intermed.',
    email: 'Email',
    password: 'Parolă',
    signIn: 'Intră în cont',
    signingIn: 'Se autentifică...',
    invalid: 'Email sau parolă incorecte.',
    logout: 'Deconectare',
  },
  pipeline: {
    title: 'Pipeline lead-uri',
    subtitle: 'Cumpărători calificați prin agentul Angrosist',
    search: 'Caută companie, produs...',
    filterStatus: 'Status',
    filterVertical: 'Vertical',
    filterAssignee: 'Responsabil',
    allStatuses: 'Toate statusurile',
    allVerticals: 'Toate verticalele',
    allAssignees: 'Toți responsabilii',
    unassigned: 'Nealocat',
    needsHuman: 'Necesită intervenție',
    empty: 'Nu există lead-uri pentru filtrele selectate.',
    loadError: 'Nu am putut încărca lead-urile.',
    prev: 'Anterior',
    next: 'Următor',
    colCompany: 'Companie',
    colProduct: 'Produs',
    colQuantity: 'Cantitate',
    colLocation: 'Locație',
    colStatus: 'Status',
    colAssignee: 'Responsabil',
    colValue: 'Valoare',
    colCreated: 'Creat',
  },
  detail: {
    notFound: 'Lead negăsit.',
    loadError: 'Nu am putut încărca lead-ul.',
    transcript: 'Transcriere conversație',
    noMessages: 'Fără mesaje.',
    extracted: 'Detalii extrase',
    company: 'Companie',
    contact: 'Contact',
    offer: 'Ofertă',
    assignee: 'Responsabil',
    sourcingRequest: 'Cerere de aprovizionare',
    roles: 'Roluri',
    vatStatus: 'Status TVA',
    caen: 'CAEN',
    country: 'Țară',
    administrators: 'Administratori',
    checkedAt: 'Verificat la',
    noVerification: 'Companie neverificată.',
    offerStatus: 'Status',
    offerValue: 'Valoare (RON)',
    offerNote: 'Notă',
    offerSaved: 'Ofertă actualizată.',
    offerError: 'Nu am putut salva oferta.',
    assignSaved: 'Responsabil actualizat.',
    assignError: 'Nu am putut aloca lead-ul.',
    assignToMe: 'Aloca-mi mie',
  },
  status: {
    new: 'Nou',
    qualifying: 'În calificare',
    needs_human: 'Necesită om',
    qualified: 'Calificat',
    offer_requested: 'Ofertă cerută',
    offer_sent: 'Ofertă trimisă',
    negotiation: 'Negociere',
    won: 'Câștigat',
    lost: 'Pierdut',
  } as Record<string, string>,
  vertical: {
    angrosist: 'Angrosist',
    palletclearance: 'PalletClearance',
    skalyou: 'SkalYou',
  } as Record<string, string>,
}

export const LEAD_STATUSES: { value: string; label: string }[] = [
  'new',
  'qualifying',
  'needs_human',
  'qualified',
  'offer_requested',
  'offer_sent',
  'negotiation',
  'won',
  'lost',
].map((value) => ({ value, label: t.status[value] ?? value }))

export const VERTICALS: { value: string; label: string }[] = [
  'angrosist',
  'palletclearance',
  'skalyou',
].map((value) => ({ value, label: t.vertical[value] ?? value }))

export function statusLabel(status: string): string {
  return t.status[status] ?? status
}

export function verticalLabel(v: string): string {
  return t.vertical[v] ?? v
}
