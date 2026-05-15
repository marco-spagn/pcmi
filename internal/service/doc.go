// Package service contiene la logica di dominio usata sia dall’API HTTP sia da gRPC:
// store/retrieve memorie, batch, admin, riassunto, ingest eventi. Dipende da repository
// e da internal/embedding per i vettori; pubblica eventi Redis dopo store tramite internal/event.
package service
