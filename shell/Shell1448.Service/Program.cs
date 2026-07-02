using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Hosting;
using Shell1448.Service;

// 14:48 Shell Service — привилегированная половина PC Shell.
// Запуск как Windows-сервис (см. shell/install-service.ps1) или консольно (дев).
var builder = Host.CreateApplicationBuilder(args);

builder.Services.Configure<ServiceConfig>(builder.Configuration.GetSection("Shell"));
builder.Services.AddSingleton<PipeServer>();
builder.Services.AddHostedService<Worker>();

builder.Services.AddWindowsService(o => o.ServiceName = "Shell1448");

var host = builder.Build();
host.Run();
