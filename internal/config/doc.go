// Package config centralizza il caricamento e la validazione di tutte le variabili d'ambiente PCMI.
// Usare Load() per caricare la configurazione e Validate() per eseguire la validazione fail-fast
// all'avvio del servizio. Ogni campo ha un valore di default documentato e le variabili obbligatorie
// vengono controllate prima che qualsiasi connessione venga aperta.
package config
