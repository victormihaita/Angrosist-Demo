import { useQuery } from '@tanstack/react-query'
import { getLeads, getLead } from '@/lib/api'

export function useLeads() {
  return useQuery({
    queryKey: ['leads'],
    queryFn: getLeads,
    refetchInterval: 30_000,
  })
}

export function useLead(id: string) {
  return useQuery({
    queryKey: ['lead', id],
    queryFn: () => getLead(id),
    enabled: !!id,
  })
}
