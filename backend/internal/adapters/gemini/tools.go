package gemini

import "github.com/google/generative-ai-go/genai"

var agentTools = []*genai.Tool{
	{
		FunctionDeclarations: []*genai.FunctionDeclaration{
			{
				Name:        "verify_company",
				Description: "Verifică o companie românească prin CUI/CIF folosind baza de date ANAF. Apelează imediat ce ai CUI-ul de la utilizator.",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"cui": {
							Type:        genai.TypeString,
							Description: "Codul unic de identificare fiscală (CUI sau CIF) al companiei, doar cifre",
						},
					},
					Required: []string{"cui"},
				},
			},
			{
				Name:        "save_lead",
				Description: "Salvează lead-ul calificat după ce toate informațiile sunt colectate și compania verificată.",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"product_name": {
							Type:        genai.TypeString,
							Description: "Produsul sau categoria de produse",
						},
						"quantity": {
							Type:        genai.TypeNumber,
							Description: "Cantitatea necesară",
						},
						"unit": {
							Type:        genai.TypeString,
							Description: "Unitatea de măsură (kg, buc, palet, camion, tonă etc.)",
						},
						"delivery_location": {
							Type:        genai.TypeString,
							Description: "Orașul sau județul pentru livrare",
						},
						"cui": {
							Type:        genai.TypeString,
							Description: "CUI-ul companiei verificate",
						},
						"company_name": {
							Type:        genai.TypeString,
							Description: "Numele companiei confirmat de ANAF (sau introdus de utilizator dacă ANAF indisponibil)",
						},
						"phone": {
							Type:        genai.TypeString,
							Description: "Numărul de telefon de contact al persoanei",
						},
						"email": {
							Type:        genai.TypeString,
							Description: "Adresa de email de contact al persoanei",
						},
					},
					Required: []string{"product_name", "quantity", "unit", "delivery_location", "cui", "company_name", "phone", "email"},
				},
			},
		},
	},
}
